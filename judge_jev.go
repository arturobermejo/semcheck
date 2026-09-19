package semcheck

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// APIKeyEnv is the variable DefaultJudge reads the key of JevJudge from,
	// the same one the SDKs of TypeSafe use.
	APIKeyEnv = "TYPESAFE_API_KEY"

	DefaultJevURL   = "https://api.typesafe.ai"
	DefaultJevModel = "jev-latest"

	jevTimeout = 30 * time.Second

	// The answers are a few hundred bytes. The limit is for whatever else
	// might be listening at the address.
	maxJevResponse = 1 << 20
)

// JevJudge asks Jev, the decision model of TypeSafe AI. It sends the code of
// every question, and the notes on its types, to the API: using it is a
// decision to share that code with a third party.
type JevJudge struct {
	APIKey string

	// URL, Model and Client default to DefaultJevURL, DefaultJevModel and a
	// client with a timeout.
	URL    string
	Model  string
	Client *http.Client
}

var _ Judge = (*JevJudge)(nil)

var defaultJevClient = &http.Client{Timeout: jevTimeout}

// jevState is what Jev reads to answer. Its field names are part of the
// prompt: the model sees them.
type jevState struct {
	Code  string   `json:"code"`
	Types []string `json:"variable_types,omitempty"`
}

type jevRequest struct {
	Model     string                 `json:"model"`
	State     jevState               `json:"state"`
	Questions map[string]jevQuestion `json:"questions"`
}

type jevQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
}

type jevResponse struct {
	Answers map[string]struct {
		// A pointer tells a missing answer from a certain no.
		Noul *float64 `json:"noul"`
	} `json:"answers"`
}

// questionID names the only question of a request.
const questionID = "answer"

// Decide makes one request per question, one after the other: every question
// comes with its own code, and a request has a single state. The first failure
// ends the batch, since the ones after it would most likely fail the same way.
func (j *JevJudge) Decide(ctx context.Context, questions []Question) ([]Decision, error) {
	if j.APIKey == "" {
		return nil, errors.New("jev: there is no API key")
	}

	decisions := make([]Decision, len(questions))

	for i, q := range questions {
		yes, err := j.ask(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("jev: question %d of %d (rule %s): %w", i+1, len(questions), q.Rule, err)
		}

		decisions[i].Yes = yes
	}

	return decisions, nil
}

func (j *JevJudge) ask(ctx context.Context, q Question) (float64, error) {
	body, err := json.Marshal(jevRequest{
		Model:     cmp.Or(j.Model, DefaultJevModel),
		State:     jevState{Code: q.Fragment, Types: q.Types},
		Questions: map[string]jevQuestion{questionID: {Type: "noul", Instructions: q.Ask}},
	})
	if err != nil {
		return 0, err
	}

	url := strings.TrimSuffix(cmp.Or(j.URL, DefaultJevURL), "/") + "/v1/systemone"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}

	req.Header.Set("Authorization", "Bearer "+j.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := j.Client
	if client == nil {
		client = defaultJevClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxJevResponse))
	if err != nil {
		return 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%s: %s", resp.Status, excerpt(data))
	}

	var answer jevResponse
	if err := json.Unmarshal(data, &answer); err != nil {
		return 0, fmt.Errorf("unexpected response: %w", err)
	}

	noul := answer.Answers[questionID].Noul
	if noul == nil {
		return 0, fmt.Errorf("the response has no answer: %s", excerpt(data))
	}

	return *noul, nil
}

// excerpt is the start of a response body, for an error message.
func excerpt(data []byte) string {
	const limit = 200

	text := strings.Join(strings.Fields(string(data)), " ")
	if len(text) > limit {
		text = text[:limit] + "…"
	}

	return text
}

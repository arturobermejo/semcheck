package semcheck

import (
	"context"
	"fmt"
	"net/http"

	"github.com/arturobermejo/semcheck/internal/jev"
)

// APIKeyEnv is the variable DefaultJudge reads the key of JevJudge from, the
// same one the SDKs of TypeSafe use.
const APIKeyEnv = "TYPESAFE_API_KEY"

// JevJudge asks Jev, the decision model of TypeSafe AI. It sends the code of
// every question, and the notes on its types, to the API: using it is a
// decision to share that code with a third party.
type JevJudge struct {
	APIKey string

	// URL, Model and Client default to the public API, its latest model and a
	// client with a timeout.
	URL    string
	Model  string
	Client *http.Client
}

var _ Judge = (*JevJudge)(nil)

// jevState is what Jev reads to answer. Its field names are part of the
// prompt: the model sees them.
type jevState struct {
	Code  string   `json:"code"`
	Types []string `json:"variable_types,omitempty"`
}

// Decide makes one request per question, one after the other: every question
// comes with its own code, and a request has a single state. The first failure
// ends the batch, since the ones after it would most likely fail the same way.
func (j *JevJudge) Decide(ctx context.Context, questions []Question) ([]Decision, error) {
	const name = "answer"

	client := &jev.Client{APIKey: j.APIKey, URL: j.URL, Model: j.Model, HTTPClient: j.Client}
	decisions := make([]Decision, len(questions))

	for i, q := range questions {
		response, err := client.Ask(ctx, jevState{q.Fragment, q.Types}, map[string]jev.Question{name: jev.Noul(q.Ask)})
		if err == nil {
			decisions[i].Yes, err = response.Noul(name)
		}

		if err != nil {
			return nil, fmt.Errorf("question %d of %d (rule %s): %w", i+1, len(questions), q.Rule, err)
		}
	}

	return decisions, nil
}

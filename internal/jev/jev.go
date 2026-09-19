// Package jev is a client for the API of Jev, the decision model of TypeSafe
// AI (https://docs.typesafe.ai). It follows the shape of the API and knows
// nothing about semcheck, so that an official client can take its place.
package jev

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
	DefaultURL   = "https://api.typesafe.ai"
	DefaultModel = "jev-latest"

	timeout = 30 * time.Second

	// The answers are a few hundred bytes. The limit is for whatever else
	// might be listening at the address.
	maxResponse = 1 << 20
)

// A Client asks questions to the API. Only APIKey is required.
type Client struct {
	APIKey string

	// URL, Model and HTTPClient default to DefaultURL, DefaultModel and a
	// client with a timeout.
	URL        string
	Model      string
	HTTPClient *http.Client
}

var defaultHTTPClient = &http.Client{Timeout: timeout}

// A Question is one of the typed questions of the API.
type Question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
}

// Noul returns a yes-or-no question. Its answer is the probability of yes.
func Noul(instructions string) Question {
	return Question{Type: "noul", Instructions: instructions}
}

type request struct {
	Model     string              `json:"model"`
	State     any                 `json:"state"`
	Questions map[string]Question `json:"questions"`
}

// A Response has the answer to every question of a request, under its name.
type Response struct {
	// Model is the version that answered, even when the request named an alias
	// such as jev-latest.
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

type Answer struct {
	Type string `json:"type"`

	// A pointer tells a missing answer from a certain no.
	Noul *float64 `json:"noul"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// A StatusError is a response of the API other than 200 OK.
type StatusError struct {
	Code   int
	Status string
	Body   string // the beginning of the body, on one line
}

func (e *StatusError) Error() string {
	if e.Body == "" {
		return "jev: " + e.Status
	}

	return "jev: " + e.Status + ": " + e.Body
}

// Ask sends one request: every question is answered about the same state,
// which is a string or anything that becomes JSON. The model sees the names of
// the fields of the state.
func (c *Client) Ask(ctx context.Context, state any, questions map[string]Question) (*Response, error) {
	if c.APIKey == "" {
		return nil, errors.New("jev: there is no API key")
	}

	body, err := json.Marshal(request{Model: cmp.Or(c.Model, DefaultModel), State: state, Questions: questions})
	if err != nil {
		return nil, fmt.Errorf("jev: %w", err)
	}

	url := strings.TrimSuffix(cmp.Or(c.URL, DefaultURL), "/") + "/v1/systemone"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jev: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := cmp.Or(c.HTTPClient, defaultHTTPClient).Do(req)
	if err != nil {
		return nil, fmt.Errorf("jev: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return nil, fmt.Errorf("jev: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{Code: resp.StatusCode, Status: resp.Status, Body: excerpt(data)}
	}

	var response Response
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("jev: unexpected response: %w", err)
	}

	return &response, nil
}

// Noul returns the probability of yes that answers the noul question name.
func (r *Response) Noul(name string) (float64, error) {
	noul := r.Answers[name].Noul
	if noul == nil {
		return 0, fmt.Errorf("jev: the response has no noul answer to %q", name)
	}

	return *noul, nil
}

func excerpt(data []byte) string {
	const limit = 200

	text := strings.Join(strings.Fields(string(data)), " ")
	if len(text) > limit {
		text = text[:limit] + "…"
	}

	return text
}

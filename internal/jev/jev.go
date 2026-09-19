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
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultURL   = "https://api.typesafe.ai"
	DefaultModel = "jev-latest"

	// DefaultMaxRetries and the waits between attempts are the ones of the
	// official SDKs.
	DefaultMaxRetries = 2

	timeout = 30 * time.Second

	firstWait = 500 * time.Millisecond
	maxWait   = 5 * time.Second
	jitter    = 0.25 // a wait is shortened by up to this much, at random

	// A server that asks for more patience than this gets none: the error is
	// worth more now than the answer then.
	maxRetryAfter = 30 * time.Second

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

	// MaxRetries is how many times a request that failed for a passing reason
	// is sent again: DefaultMaxRetries if zero, never if negative.
	MaxRetries int

	wait func(context.Context, time.Duration) error // sleep, unless a test sets it
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

	// RetryAfter is how long the server asked to wait, if it did.
	RetryAfter time.Duration
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
//
// A request that fails for a passing reason (the API is overloaded or asks to
// slow down, the connection breaks) is sent again after a wait, up to
// MaxRetries times.
func (c *Client) Ask(ctx context.Context, state any, questions map[string]Question) (*Response, error) {
	if c.APIKey == "" {
		return nil, errors.New("jev: there is no API key")
	}

	body, err := json.Marshal(request{Model: cmp.Or(c.Model, DefaultModel), State: state, Questions: questions})
	if err != nil {
		return nil, fmt.Errorf("jev: %w", err)
	}

	retries := max(cmp.Or(c.MaxRetries, DefaultMaxRetries), 0)

	for attempt := 1; ; attempt++ {
		response, err := c.send(ctx, body)
		if err == nil {
			return response, nil
		}

		wait, ok := retryIn(err, attempt)
		if !ok || attempt > retries {
			if attempt > 1 {
				err = fmt.Errorf("%w (after %d attempts)", err, attempt)
			}

			return nil, err
		}

		pause := sleep
		if c.wait != nil {
			pause = c.wait
		}

		if err := pause(ctx, wait); err != nil {
			return nil, fmt.Errorf("jev: %w", err)
		}
	}
}

func (c *Client) send(ctx context.Context, body []byte) (*Response, error) {
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
		return nil, &StatusError{
			Code:       resp.StatusCode,
			Status:     statusLine(resp),
			Body:       excerpt(data),
			RetryAfter: retryAfter(resp.Header, time.Now()),
		}
	}

	var response Response
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("jev: unexpected response: %w", err)
	}

	return &response, nil
}

// statusLine names the one status of the API that net/http does not know,
// which would read "529 status code 529".
func statusLine(resp *http.Response) string {
	if resp.StatusCode == 529 {
		return "529 Overloaded"
	}

	return resp.Status
}

// retryIn reports whether the failure of an attempt may pass, and how long to
// wait before the next one.
func retryIn(err error, attempt int) (time.Duration, bool) {
	var status *StatusError

	switch {
	case errors.As(err, &status):
		passing := status.Code == http.StatusRequestTimeout || status.Code == http.StatusTooManyRequests || status.Code >= 500
		if !passing || status.RetryAfter > maxRetryAfter {
			return 0, false
		}

		if status.RetryAfter > 0 {
			return status.RetryAfter, true
		}
	case isTimeout(err):
		// The model answers in under a second. After waiting for as long as
		// the client allows, another attempt would most likely cost the same.
		return 0, false
	case !isConnection(err):
		// A response that made no sense would make no more the second time.
		return 0, false
	}

	// 0.5 s, 1 s, 2 s... up to maxWait.
	wait := min(firstWait<<(attempt-1), maxWait)

	return wait - time.Duration(rand.Float64()*jitter*float64(wait)), true
}

func isTimeout(err error) bool {
	var netErr net.Error

	return errors.As(err, &netErr) && netErr.Timeout()
}

// isConnection reports whether the request, or its response, got lost on the
// way. Every error of http.Client.Do is a net.Error.
func isConnection(err error) bool {
	var netErr net.Error

	return errors.As(err, &netErr) || errors.Is(err, io.ErrUnexpectedEOF)
}

// retryAfter reads how long the server asked to wait: in milliseconds, in
// seconds or until a date.
func retryAfter(header http.Header, now time.Time) time.Duration {
	if ms, err := strconv.ParseFloat(header.Get("Retry-After-Ms"), 64); err == nil && ms > 0 {
		return time.Duration(ms * float64(time.Millisecond))
	}

	value := header.Get("Retry-After")

	if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds > 0 {
		return time.Duration(seconds * float64(time.Second))
	}

	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date.Sub(now)
	}

	return 0
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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

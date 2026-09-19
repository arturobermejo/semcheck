package jev

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testKey = "sk-test-0123456789"

// fakeAPI answers with respond, and keeps the last request.
type fakeAPI struct {
	*httptest.Server

	requests atomic.Int32
	header   http.Header
	path     string
	body     string
}

func newFakeAPI(t *testing.T, respond http.HandlerFunc) *fakeAPI {
	t.Helper()

	api := &fakeAPI{}

	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.requests.Add(1)
		api.header, api.path = r.Header, r.Method+" "+r.URL.Path

		data, _ := io.ReadAll(r.Body)
		api.body = string(data)

		respond(w, r)
	}))
	t.Cleanup(api.Close)

	return api
}

func (api *fakeAPI) client() *Client {
	return &Client{APIKey: testKey, URL: api.URL}
}

func respondWith(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, body) }
}

const twoAnswers = `{
	"model": "jev-1.13.0",
	"answers": {"pii": {"type": "noul", "noul": 0.97}, "secrets": {"type": "noul", "noul": 0}},
	"usage": {"input_tokens": 80, "output_tokens": 0}
}`

func TestAsk(t *testing.T) {
	api := newFakeAPI(t, respondWith(twoAnswers))

	state := map[string]any{"code": "log.Print(u)"}
	questions := map[string]Question{"pii": Noul("Personal data?"), "secrets": Noul("Secrets?")}

	response, err := api.client().Ask(context.Background(), state, questions)
	if err != nil {
		t.Fatal(err)
	}

	if api.path != "POST /v1/systemone" {
		t.Errorf("request = %s", api.path)
	}

	if got := api.header.Get("Authorization"); got != "Bearer "+testKey {
		t.Errorf("Authorization = %q", got)
	}

	if got := api.header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}

	want := `{"model":"jev-latest","state":{"code":"log.Print(u)"},"questions":{` +
		`"pii":{"type":"noul","instructions":"Personal data?"},` +
		`"secrets":{"type":"noul","instructions":"Secrets?"}}}`
	if api.body != want {
		t.Errorf("body:\n got %s\nwant %s", api.body, want)
	}

	if response.Model != "jev-1.13.0" || response.Usage.InputTokens != 80 {
		t.Errorf("response = %+v", response)
	}

	if pii, err := response.Noul("pii"); err != nil || pii != 0.97 {
		t.Errorf("pii = %v, %v", pii, err)
	}

	// A certain no is an answer, unlike a question that was not answered.
	if secrets, err := response.Noul("secrets"); err != nil || secrets != 0 {
		t.Errorf("secrets = %v, %v", secrets, err)
	}

	if _, err := response.Noul("other"); err == nil {
		t.Error("want an error for a question without an answer")
	}
}

func TestAskStringState(t *testing.T) {
	api := newFakeAPI(t, respondWith(twoAnswers))

	if _, err := api.client().Ask(context.Background(), "some text", nil); err != nil {
		t.Fatal(err)
	}

	var body struct{ State string }
	if err := json.Unmarshal([]byte(api.body), &body); err != nil || body.State != "some text" {
		t.Errorf("body = %s", api.body)
	}
}

func TestAskModelAndURL(t *testing.T) {
	api := newFakeAPI(t, respondWith(twoAnswers))

	client := &Client{APIKey: testKey, URL: api.URL + "/", Model: "jev-1.13.0"}
	if _, err := client.Ask(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}

	if api.path != "POST /v1/systemone" || !strings.Contains(api.body, `"model":"jev-1.13.0"`) {
		t.Errorf("request = %s %s", api.path, api.body)
	}
}

func TestAskErrors(t *testing.T) {
	tests := []struct {
		name    string
		respond http.HandlerFunc
		want    string
		code    int
	}{
		{
			"unauthorized",
			func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, `{"error": "invalid API key"}`, http.StatusUnauthorized)
			},
			`jev: 401 Unauthorized: {"error": "invalid API key"}`,
			401,
		},
		{
			"rate limit without a body",
			func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) },
			"jev: 429 Too Many Requests",
			429,
		},
		{
			"long body",
			func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, strings.Repeat("x", 5000), http.StatusInternalServerError)
			},
			"jev: 500 Internal Server Error: " + strings.Repeat("x", 200) + "…",
			500,
		},
		{"not json", respondWith("<html>"), "unexpected response", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, tt.respond)

			response, err := api.client().Ask(context.Background(), "", nil)
			if err == nil {
				t.Fatalf("response = %+v, want an error", response)
			}

			if tt.code != 0 && err.Error() != tt.want || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want %q", err, tt.want)
			}

			var status *StatusError
			if errors.As(err, &status) != (tt.code != 0) || tt.code != 0 && status.Code != tt.code {
				t.Errorf("error = %#v, want a StatusError only for status %d", err, tt.code)
			}

			if strings.Contains(err.Error(), testKey) {
				t.Errorf("error %q shows the API key", err)
			}
		})
	}
}

func TestAskUnreachable(t *testing.T) {
	api := newFakeAPI(t, respondWith(twoAnswers))
	client := api.client()

	api.Close()

	_, err := client.Ask(context.Background(), "", nil)
	if err == nil || strings.Contains(err.Error(), testKey) {
		t.Errorf("error = %v, want one that does not show the API key", err)
	}
}

func TestAskWithoutKey(t *testing.T) {
	api := newFakeAPI(t, respondWith(twoAnswers))

	_, err := (&Client{URL: api.URL}).Ask(context.Background(), "", nil)
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("error = %v, want one about the API key", err)
	}

	if n := api.requests.Load(); n != 0 {
		t.Errorf("%d requests, want none", n)
	}
}

func TestAskCanceled(t *testing.T) {
	api := newFakeAPI(t, respondWith(twoAnswers))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := api.client().Ask(ctx, "", nil); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

package jev

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

// client returns one that does not retry.
func (api *fakeAPI) client() *Client {
	return &Client{APIKey: testKey, URL: api.URL, MaxRetries: -1}
}

// retrying returns a client that records its waits instead of sleeping.
func (api *fakeAPI) retrying(waits *[]time.Duration) *Client {
	return &Client{APIKey: testKey, URL: api.URL, wait: func(_ context.Context, d time.Duration) error {
		*waits = append(*waits, d)

		return nil
	}}
}

// failing answers with respond the first n requests, and with two answers
// after that.
func failing(n int32, respond http.HandlerFunc) http.HandlerFunc {
	var served atomic.Int32

	return func(w http.ResponseWriter, r *http.Request) {
		if served.Add(1) <= n {
			respond(w, r)

			return
		}

		respondWith(twoAnswers)(w, r)
	}
}

func status(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }
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

	client := &Client{APIKey: testKey, URL: api.URL + "/", Model: "jev-1.13.0", MaxRetries: -1}
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

func TestAskRetries(t *testing.T) {
	tests := []struct {
		name     string
		failures int32
		respond  http.HandlerFunc
		requests int32 // 0: it ends in an error
	}{
		{"overloaded once", 1, status(529), 2},
		{"rate limit twice", 2, status(http.StatusTooManyRequests), 3},
		{"request timeout", 1, status(http.StatusRequestTimeout), 2},
		{"bad gateway", 1, status(http.StatusBadGateway), 2},
		{"three times is too many", 3, status(529), 0},
		{"unauthorized", 1, status(http.StatusUnauthorized), 0},
		{"unprocessable", 1, status(http.StatusUnprocessableEntity), 0},
		{"not json", 1, respondWith("<html>"), 0},
		{
			"connection cut before the response", 1,
			func(w http.ResponseWriter, _ *http.Request) {
				conn, _, _ := w.(http.Hijacker).Hijack()
				conn.Close()
			},
			2,
		},
		{
			"connection cut in the middle of the response", 1,
			func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Length", "1000")
				io.WriteString(w, `{"answers":`)
			},
			2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, failing(tt.failures, tt.respond))

			var waits []time.Duration

			response, err := api.retrying(&waits).Ask(context.Background(), "", nil)

			if tt.requests == 0 {
				if err == nil {
					t.Fatalf("response = %+v, want an error", response)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if n := api.requests.Load(); n != tt.requests || len(waits) != int(n)-1 {
				t.Errorf("%d requests and %d waits, want %d requests", n, len(waits), tt.requests)
			}
		})
	}
}

func TestAskGivesUp(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "overloaded", 529) })

	var waits []time.Duration

	_, err := api.retrying(&waits).Ask(context.Background(), "", nil)
	if err == nil || err.Error() != "jev: 529 Overloaded: overloaded (after 3 attempts)" {
		t.Errorf("error = %v", err)
	}

	var status *StatusError
	if !errors.As(err, &status) || status.Code != 529 {
		t.Errorf("error = %#v, want it to wrap the StatusError", err)
	}

	if n := api.requests.Load(); n != 1+DefaultMaxRetries {
		t.Errorf("%d requests, want %d", n, 1+DefaultMaxRetries)
	}

	// 0.5 s and 1 s, each shortened by up to a quarter.
	for i, base := range []time.Duration{500 * time.Millisecond, time.Second} {
		if len(waits) != 2 || waits[i] > base || waits[i] < base*3/4 {
			t.Errorf("waits = %v, want 2 of about 0.5s and 1s", waits)

			break
		}
	}
}

func TestAskWaitsGrowUpToALimit(t *testing.T) {
	api := newFakeAPI(t, status(529))

	var waits []time.Duration

	client := api.retrying(&waits)
	client.MaxRetries = 6

	if _, err := client.Ask(context.Background(), "", nil); err == nil {
		t.Fatal("want an error")
	}

	if len(waits) != 6 {
		t.Fatalf("waits = %v, want 6", waits)
	}

	for i, base := range []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second, maxWait, maxWait} {
		if waits[i] > base || waits[i] < base*3/4 {
			t.Errorf("wait %d = %v, want from %v to %v", i+1, waits[i], base*3/4, base)
		}
	}

	if slices.Equal(waits[4:5], waits[5:6]) {
		t.Errorf("waits = %v: two equal waits in a row, want jitter", waits)
	}
}

func TestAskRetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		header string
		value  string
		want   time.Duration // 0: it does not retry
	}{
		{"seconds", "Retry-After", "2", 2 * time.Second},
		{"a fraction", "Retry-After", "0.25", 250 * time.Millisecond},
		{"milliseconds", "Retry-After-Ms", "40", 40 * time.Millisecond},
		{"date", "Retry-After", time.Now().Add(10 * time.Second).UTC().Format(http.TimeFormat), 10 * time.Second},
		{"too long", "Retry-After", "3600", 0},
		{"nonsense", "Retry-After", "soon", 500 * time.Millisecond},
		{"negative", "Retry-After", "-5", 500 * time.Millisecond},
		{"date in the past", "Retry-After", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat), 500 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, failing(1, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set(tt.header, tt.value)
				w.WriteHeader(http.StatusTooManyRequests)
			}))

			var waits []time.Duration

			_, err := api.retrying(&waits).Ask(context.Background(), "", nil)

			if tt.want == 0 {
				if err == nil || len(waits) != 0 || api.requests.Load() != 1 {
					t.Errorf("error = %v after waiting %v, want it to give up at once", err, waits)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			// A date has the precision of a second, and the default wait has jitter.
			if len(waits) != 1 || waits[0] > tt.want || waits[0] < tt.want*3/4-time.Second {
				t.Errorf("waits = %v, want %v", waits, tt.want)
			}
		})
	}
}

func TestAskDoesNotRetryATimeout(t *testing.T) {
	release := make(chan struct{})

	api := newFakeAPI(t, func(http.ResponseWriter, *http.Request) { <-release })

	defer close(release)

	var waits []time.Duration

	client := api.retrying(&waits)
	client.HTTPClient = &http.Client{Timeout: 50 * time.Millisecond}

	if _, err := client.Ask(context.Background(), "", nil); err == nil {
		t.Fatal("want an error")
	}

	if n := api.requests.Load(); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestAskStopsWaitingWhenCanceled(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "20")
		w.WriteHeader(529)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()

	// The real sleep.
	_, err := (&Client{APIKey: testKey, URL: api.URL}).Ask(ctx, "", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}

	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("it took %v", took)
	}

	if n := api.requests.Load(); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestSleep(t *testing.T) {
	start := time.Now()

	if err := sleep(context.Background(), 30*time.Millisecond); err != nil || time.Since(start) < 30*time.Millisecond {
		t.Errorf("sleep = %v after %v", err, time.Since(start))
	}
}

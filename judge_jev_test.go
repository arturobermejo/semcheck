package semcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const testKey = "sk-test-0123456789"

// A jevCall is a request as fakeJev got it.
type jevCall struct {
	Model string `json:"model"`
	State struct {
		Code  string   `json:"code"`
		Types []string `json:"variable_types"`
	} `json:"state"`
	Questions map[string]struct {
		Type         string `json:"type"`
		Instructions string `json:"instructions"`
	} `json:"questions"`

	raw map[string]any
}

// fakeJev is an API that gives every question the probability of answer.
type fakeJev struct {
	*httptest.Server

	// handle, if set, runs first. It may answer the request itself, and then
	// returns true.
	handle func(w http.ResponseWriter, call jevCall) bool

	mu    sync.Mutex
	calls []jevCall
}

func newFakeJev(t *testing.T, answer func(code, ask string) float64) *fakeJev {
	t.Helper()

	api := &fakeJev{}

	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call jevCall

		body := json.NewDecoder(r.Body)
		if err := body.Decode(&call.raw); err != nil {
			t.Errorf("the request is not JSON: %v", err)
		}

		data, _ := json.Marshal(call.raw)
		_ = json.Unmarshal(data, &call)

		api.mu.Lock()
		api.calls = append(api.calls, call)
		api.mu.Unlock()

		if api.handle != nil && api.handle(w, call) {
			return
		}

		answers := map[string]any{}
		for name, q := range call.Questions {
			answers[name] = map[string]any{"type": "noul", "noul": answer(call.State.Code, q.Instructions)}
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-1.13.0", "answers": answers,
			"usage": map[string]int{"input_tokens": fakeJevTokens, "output_tokens": 1},
		})
	}))
	t.Cleanup(api.Close)

	return api
}

// fakeJevTokens is what the fake API bills for every request.
const fakeJevTokens = 100

// judge returns one that does not retry.
func (api *fakeJev) judge() *JevJudge {
	return &JevJudge{APIKey: testKey, URL: api.URL, MaxRetries: -1}
}

func (api *fakeJev) requests() []jevCall {
	api.mu.Lock()
	defer api.mu.Unlock()

	return slices.Clone(api.calls)
}

func always(p float64) func(string, string) float64 {
	return func(string, string) float64 { return p }
}

func TestJevJudgeRequest(t *testing.T) {
	api := newFakeJev(t, always(0.97))

	question := Question{
		Rule:     "no-pii-in-logs",
		Ask:      "Does this log include personal data?",
		Fragment: `log.Printf("%+v", u)`,
		Types:    []string{"u: *User{Email string}"},
	}

	decisions, err := api.judge().Decide(context.Background(), []Question{question})
	if err != nil {
		t.Fatal(err)
	}

	if len(decisions) != 1 || decisions[0].Yes != 0.97 || decisions[0].Err != nil {
		t.Errorf("decisions = %+v, want a single 0.97", decisions)
	}

	want := map[string]any{
		"model": "jev-latest",
		"state": map[string]any{
			"code":           `log.Printf("%+v", u)`,
			"variable_types": []any{"u: *User{Email string}"},
		},
		"questions": map[string]any{
			"q1": map[string]any{"type": "noul", "instructions": "Does this log include personal data?"},
		},
	}

	got, _ := json.Marshal(api.requests()[0].raw)
	if wantJSON, _ := json.Marshal(want); string(got) != string(wantJSON) {
		t.Errorf("body:\n got %s\nwant %s\nIf the change is meant, jevPrompt has to change too: "+
			"the cache holds answers to the old request", got, wantJSON)
	}
}

func TestJevJudgeIdentity(t *testing.T) {
	tests := []struct {
		judge *JevJudge
		want  string
	}{
		{&JevJudge{}, "jev 1 jev-latest"},
		{&JevJudge{Model: "jev-1.13.0"}, "jev 1 jev-1.13.0"},
		// Who asks, where and how often does not change the answers.
		{&JevJudge{APIKey: testKey, URL: "http://localhost", Concurrency: 2, MaxRetries: 5}, "jev 1 jev-latest"},
	}

	for _, tt := range tests {
		if got := tt.judge.identity(); got != tt.want {
			t.Errorf("identity = %q, want %q", got, tt.want)
		}
	}
}

func TestJevJudgeWithoutTypeNotes(t *testing.T) {
	api := newFakeJev(t, always(0))

	decisions, err := api.judge().Decide(context.Background(), questions("x := 1"))
	if err != nil || decisions[0].Yes != 0 || decisions[0].Err != nil {
		t.Fatalf("Decide = %+v, %v; want a certain no", decisions, err)
	}

	state := api.requests()[0].raw["state"].(map[string]any)
	if _, ok := state["variable_types"]; ok {
		t.Errorf("state = %v, want no variable_types", state)
	}
}

func TestJevJudgeModel(t *testing.T) {
	api := newFakeJev(t, always(0.5))

	judge := api.judge()
	judge.Model = "jev-1.13.0"

	if _, err := judge.Decide(context.Background(), questions("a")); err != nil {
		t.Fatal(err)
	}

	if got := api.requests()[0].Model; got != "jev-1.13.0" {
		t.Errorf("model = %v", got)
	}
}

// The questions about the same code, with the same notes, share a request.
func TestJevJudgeGroupsByState(t *testing.T) {
	api := newFakeJev(t, func(code, ask string) float64 {
		return map[string]float64{"a pii?": 0.1, "a level?": 0.2, "b pii?": 0.3, "b level?": 0.4, "a other?": 0.5}[code+" "+ask]
	})

	batch := []Question{
		{Rule: "pii", Ask: "pii?", Fragment: "a", Types: []string{"u: User"}},
		{Rule: "pii", Ask: "pii?", Fragment: "b"},
		{Rule: "level", Ask: "level?", Fragment: "a", Types: []string{"u: User"}},
		{Rule: "level", Ask: "level?", Fragment: "b"},
		// The same code, but not the same notes: not the same state.
		{Rule: "other", Ask: "other?", Fragment: "a", Types: []string{"u: Admin"}},
	}

	decisions, err := api.judge().Decide(context.Background(), batch)
	if err != nil {
		t.Fatal(err)
	}

	var got []float64
	for _, d := range decisions {
		got = append(got, d.Yes)
	}

	if want := []float64{0.1, 0.3, 0.2, 0.4, 0.5}; !slices.Equal(got, want) {
		t.Errorf("decisions = %v, want %v", got, want)
	}

	var sizes []string
	for _, call := range api.requests() {
		sizes = append(sizes, fmt.Sprintf("%s %v: %d", call.State.Code, call.State.Types, len(call.Questions)))
	}

	slices.Sort(sizes)

	if want := []string{"a [u: Admin]: 1", "a [u: User]: 2", "b []: 2"}; !slices.Equal(sizes, want) {
		t.Errorf("requests = %q, want %q", sizes, want)
	}
}

// inFlight makes every request wait until `wait` of them are being served at
// once, and keeps the most it has seen.
type inFlight struct {
	wait int

	mu      sync.Mutex
	now     int
	most    int
	reached chan struct{}
}

func newInFlight(wait int) *inFlight {
	return &inFlight{wait: wait, reached: make(chan struct{})}
}

func (f *inFlight) handle(http.ResponseWriter, jevCall) bool {
	f.mu.Lock()
	f.now++
	f.most = max(f.most, f.now)

	if f.now == f.wait {
		select {
		case <-f.reached:
		default:
			close(f.reached)
		}
	}
	f.mu.Unlock()

	select {
	case <-f.reached:
	case <-time.After(5 * time.Second):
	}

	f.mu.Lock()
	f.now--
	f.mu.Unlock()

	return false
}

func letters(n int) []string {
	fragments := make([]string, n)
	for i := range fragments {
		fragments[i] = string(rune('a' + i))
	}

	return fragments
}

func TestJevJudgeConcurrency(t *testing.T) {
	tests := []struct {
		name        string
		concurrency int
		want        int
	}{
		{"limit", 3, 3},
		{"default", 0, DefaultJevConcurrency},
		{"nonsense", -1, DefaultJevConcurrency},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flight := newInFlight(tt.want)

			api := newFakeJev(t, always(1))
			api.handle = flight.handle

			judge := api.judge()
			judge.Concurrency = tt.concurrency

			decisions, err := judge.Decide(context.Background(), questions(letters(20)...))
			if err != nil || len(decisions) != 20 {
				t.Fatalf("Decide = %+v, %v", decisions, err)
			}

			for i, d := range decisions {
				if d.Yes != 1 || d.Err != nil {
					t.Errorf("decision %d = %+v", i, d)
				}
			}

			if flight.most != tt.want {
				t.Errorf("%d requests in flight at most, want %d", flight.most, tt.want)
			}
		})
	}
}

// Drivers call Decide for several packages at once.
func TestJevJudgeConcurrencyIsShared(t *testing.T) {
	flight := newInFlight(4)

	api := newFakeJev(t, always(1))
	api.handle = flight.handle

	judge := api.judge()
	judge.Concurrency = 4

	var wg sync.WaitGroup

	for range 5 {
		wg.Go(func() {
			if _, err := judge.Decide(context.Background(), questions(letters(10)...)); err != nil {
				t.Error(err)
			}
		})
	}

	wg.Wait()

	if flight.most != 4 {
		t.Errorf("%d requests in flight at most, want 4", flight.most)
	}
}

func TestJevJudgeStopsAtTheFirstFailure(t *testing.T) {
	failures := map[string]func(w http.ResponseWriter){
		"status":    func(w http.ResponseWriter) { http.Error(w, "overloaded", 529) },
		"not json":  func(w http.ResponseWriter) { fmt.Fprint(w, "<html>") },
		"no answer": func(w http.ResponseWriter) { fmt.Fprint(w, `{"answers": {}}`) },
	}

	for name, fail := range failures {
		t.Run(name, func(t *testing.T) {
			api := newFakeJev(t, always(1))
			api.handle = func(w http.ResponseWriter, call jevCall) bool {
				if call.State.Code == "c" {
					fail(w)
				}

				return call.State.Code == "c"
			}

			// One at a time, requests go in the order of the questions.
			judge := api.judge()
			judge.Concurrency = 1

			decisions, err := judge.Decide(context.Background(), questions(letters(6)...))
			if err != nil || len(decisions) != 6 {
				t.Fatalf("Decide = %+v, %v; want six decisions", decisions, err)
			}

			for i, d := range decisions {
				switch {
				case i < 2 && (d.Yes != 1 || d.Err != nil):
					t.Errorf("decision %d = %+v, want the answer it got before the failure", i, d)
				case i >= 2 && (d.Err == nil || !strings.HasPrefix(d.Err.Error(), "jev: ") || strings.Contains(d.Err.Error(), "canceled")):
					t.Errorf("decision %d = %+v, want the failure of the request", i, d)
				}
			}

			if n := len(api.requests()); n != 3 {
				t.Errorf("%d requests, want none after the failure", n)
			}
		})
	}
}

func TestJevJudgeFailureInParallel(t *testing.T) {
	api := newFakeJev(t, always(1))
	api.handle = func(w http.ResponseWriter, call jevCall) bool {
		if call.State.Code == "k" {
			http.Error(w, "bad key", http.StatusUnauthorized)
		}

		return call.State.Code == "k"
	}

	decisions, err := api.judge().Decide(context.Background(), questions(letters(26)...))
	if err != nil {
		t.Fatal(err)
	}

	for i, d := range decisions {
		answered := d.Err == nil && d.Yes == 1
		failed := d.Err != nil && strings.Contains(d.Err.Error(), "401 Unauthorized")

		if !answered && !failed || i == 10 && !failed {
			t.Errorf("decision %d = %+v, want an answer or the 401", i, d)
		}
	}
}

func TestJevJudgeRetries(t *testing.T) {
	var failed sync.Map

	api := newFakeJev(t, always(0.7))
	api.handle = func(w http.ResponseWriter, call jevCall) bool {
		_, seen := failed.LoadOrStore(call.State.Code, true)
		if !seen {
			w.Header().Set("Retry-After-Ms", "1")
			w.WriteHeader(http.StatusTooManyRequests)
		}

		return !seen
	}

	judge := api.judge()
	judge.MaxRetries = 0 // the default

	decisions, err := judge.Decide(context.Background(), questions(letters(5)...))
	if err != nil {
		t.Fatal(err)
	}

	for i, d := range decisions {
		if d.Yes != 0.7 || d.Err != nil {
			t.Errorf("decision %d = %+v, want the answer of the second attempt", i, d)
		}
	}

	if n := len(api.requests()); n != 10 {
		t.Errorf("%d requests, want every one of the 5 sent twice", n)
	}
}

func TestJevJudgeWithoutKey(t *testing.T) {
	api := newFakeJev(t, always(1))

	decisions, err := (&JevJudge{URL: api.URL}).Decide(context.Background(), questions("a", "b"))
	if err != nil || len(decisions) != 2 {
		t.Fatalf("Decide = %+v, %v", decisions, err)
	}

	for i, d := range decisions {
		if d.Err == nil || !strings.Contains(d.Err.Error(), "API key") {
			t.Errorf("decision %d = %+v, want an error about the API key", i, d)
		}
	}

	if n := len(api.requests()); n != 0 {
		t.Errorf("%d requests, want none", n)
	}
}

func TestJevJudgeCanceled(t *testing.T) {
	api := newFakeJev(t, always(1))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	decisions, err := api.judge().Decide(ctx, questions("a", "b"))
	if err != nil || len(decisions) != 2 || decisions[0].Err == nil || decisions[1].Err == nil {
		t.Errorf("Decide = %+v, %v; want two decisions with an error", decisions, err)
	}

	if n := len(api.requests()); n != 0 {
		t.Errorf("%d requests, want none", n)
	}
}

func TestJevJudgeNoQuestions(t *testing.T) {
	decisions, err := (&JevJudge{APIKey: testKey}).Decide(context.Background(), nil)
	if err != nil || len(decisions) != 0 {
		t.Errorf("Decide = %+v, %v", decisions, err)
	}
}

// consult is what stands between a JevJudge and the analysis.
func TestJevJudgeAnswerOutOfRange(t *testing.T) {
	api := newFakeJev(t, always(1.5))

	if _, err := consult(context.Background(), api.judge(), questions("a")); err == nil {
		t.Error("want an error for a probability of 1.5")
	}
}

func TestJevJudgeCountsTheTokensItIsBilled(t *testing.T) {
	api := newFakeJev(t, always(0.5))
	judge := api.judge()

	if tokens, ok := judge.billedTokens(); tokens != 0 || !ok {
		t.Fatalf("billedTokens = %d, %v before asking", tokens, ok)
	}

	// Two requests: the first two questions are about the same code.
	batch := []Question{{Ask: "one?", Fragment: "a"}, {Ask: "two?", Fragment: "a"}, {Ask: "one?", Fragment: "b"}}

	for range 2 {
		if _, err := judge.Decide(context.Background(), batch); err != nil {
			t.Fatal(err)
		}
	}

	if tokens, _ := judge.billedTokens(); tokens != 4*fakeJevTokens {
		t.Errorf("billedTokens = %d, want the %d of four requests", tokens, 4*fakeJevTokens)
	}
}

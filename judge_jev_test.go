package semcheck

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testKey = "sk-test-0123456789"

// fakeJev is an API that answers with respond, and keeps the last request.
type fakeJev struct {
	*httptest.Server

	requests atomic.Int32
	header   http.Header
	path     string
	body     map[string]any
}

func newFakeJev(t *testing.T, respond func(w http.ResponseWriter, body map[string]any)) *fakeJev {
	t.Helper()

	api := &fakeJev{}

	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.requests.Add(1)
		api.header, api.path = r.Header, r.Method+" "+r.URL.Path

		data, _ := io.ReadAll(r.Body)

		api.body = nil
		if err := json.Unmarshal(data, &api.body); err != nil {
			t.Errorf("the request is not JSON: %v\n%s", err, data)
		}

		respond(w, api.body)
	}))
	t.Cleanup(api.Close)

	return api
}

func (api *fakeJev) judge() *JevJudge {
	return &JevJudge{APIKey: testKey, URL: api.URL}
}

func answerWith(noul string) func(http.ResponseWriter, map[string]any) {
	return func(w http.ResponseWriter, _ map[string]any) {
		io.WriteString(w, `{"model": "jev-1.13.0", "answers": {"answer": {"type": "noul", "noul": `+noul+`}}, "usage": {"input_tokens": 80}}`)
	}
}

func TestJevJudgeRequest(t *testing.T) {
	api := newFakeJev(t, answerWith("0.97"))

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

	if api.path != "POST /v1/systemone" {
		t.Errorf("request = %s", api.path)
	}

	if got := api.header.Get("Authorization"); got != "Bearer "+testKey {
		t.Errorf("Authorization = %q", got)
	}

	if got := api.header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}

	want := map[string]any{
		"model": "jev-latest",
		"state": map[string]any{
			"code":           `log.Printf("%+v", u)`,
			"variable_types": []any{"u: *User{Email string}"},
		},
		"questions": map[string]any{
			"answer": map[string]any{"type": "noul", "instructions": "Does this log include personal data?"},
		},
	}

	got, _ := json.Marshal(api.body)
	if wantJSON, _ := json.Marshal(want); string(got) != string(wantJSON) {
		t.Errorf("body:\n got %s\nwant %s", got, wantJSON)
	}
}

func TestJevJudgeWithoutTypeNotes(t *testing.T) {
	api := newFakeJev(t, answerWith("0"))

	decisions, err := api.judge().Decide(context.Background(), questions("x := 1"))
	if err != nil || decisions[0].Yes != 0 {
		t.Fatalf("Decide = %+v, %v; want a certain no", decisions, err)
	}

	state := api.body["state"].(map[string]any)
	if _, ok := state["variable_types"]; ok {
		t.Errorf("state = %v, want no variable_types", state)
	}
}

func TestJevJudgeModelAndURL(t *testing.T) {
	api := newFakeJev(t, answerWith("0.5"))

	judge := &JevJudge{APIKey: testKey, URL: api.URL + "/", Model: "jev-1.13.0"}
	if _, err := judge.Decide(context.Background(), questions("a")); err != nil {
		t.Fatal(err)
	}

	if api.path != "POST /v1/systemone" || api.body["model"] != "jev-1.13.0" {
		t.Errorf("request = %s with model %v", api.path, api.body["model"])
	}
}

func TestJevJudgeKeepsTheOrder(t *testing.T) {
	api := newFakeJev(t, func(w http.ResponseWriter, body map[string]any) {
		code := body["state"].(map[string]any)["code"].(string)
		answerWith(map[string]string{"a": "0.1", "b": "0.2", "c": "0.3"}[code])(w, body)
	})

	decisions, err := api.judge().Decide(context.Background(), questions("a", "b", "c"))
	if err != nil {
		t.Fatal(err)
	}

	if len(decisions) != 3 || decisions[0].Yes != 0.1 || decisions[1].Yes != 0.2 || decisions[2].Yes != 0.3 {
		t.Errorf("decisions = %+v", decisions)
	}

	if n := api.requests.Load(); n != 3 {
		t.Errorf("%d requests, want one per question", n)
	}
}

func TestJevJudgeErrors(t *testing.T) {
	tests := []struct {
		name    string
		respond func(http.ResponseWriter, map[string]any)
		want    string
	}{
		{
			"unauthorized",
			func(w http.ResponseWriter, _ map[string]any) {
				http.Error(w, `{"error": "invalid API key"}`, http.StatusUnauthorized)
			},
			`401 Unauthorized: {"error": "invalid API key"}`,
		},
		{
			"rate limit",
			func(w http.ResponseWriter, _ map[string]any) { w.WriteHeader(http.StatusTooManyRequests) },
			"429 Too Many Requests",
		},
		{
			"long body",
			func(w http.ResponseWriter, _ map[string]any) {
				http.Error(w, strings.Repeat("x", 5000), http.StatusInternalServerError)
			},
			strings.Repeat("x", 200) + "…",
		},
		{"not json", func(w http.ResponseWriter, _ map[string]any) { io.WriteString(w, "<html>") }, "unexpected response"},
		{"no answers", func(w http.ResponseWriter, _ map[string]any) { io.WriteString(w, `{"answers": {}}`) }, "no answer"},
		{
			"another kind of answer",
			func(w http.ResponseWriter, _ map[string]any) {
				io.WriteString(w, `{"answers": {"answer": {"type": "choice", "choice": "yes"}}}`)
			},
			"no answer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeJev(t, tt.respond)

			decisions, err := api.judge().Decide(context.Background(), questions("a", "b", "c"))
			if err == nil {
				t.Fatalf("decisions = %+v, want an error", decisions)
			}

			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q", err, tt.want)
			}

			if !strings.Contains(err.Error(), "question 1 of 3 (rule no-pii-in-logs)") {
				t.Errorf("error %q does not say which question failed", err)
			}

			if strings.Contains(err.Error(), testKey) {
				t.Errorf("error %q shows the API key", err)
			}

			if n := api.requests.Load(); n != 1 {
				t.Errorf("%d requests, want it to stop at the first failure", n)
			}
		})
	}
}

func TestJevJudgeUnreachable(t *testing.T) {
	api := newFakeJev(t, answerWith("1"))
	judge := api.judge()

	api.Close()

	_, err := judge.Decide(context.Background(), questions("a"))
	if err == nil || strings.Contains(err.Error(), testKey) {
		t.Errorf("error = %v, want one that does not show the API key", err)
	}
}

func TestJevJudgeWithoutKey(t *testing.T) {
	api := newFakeJev(t, answerWith("1"))

	_, err := (&JevJudge{URL: api.URL}).Decide(context.Background(), questions("a"))
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("error = %v, want one about the API key", err)
	}

	if n := api.requests.Load(); n != 0 {
		t.Errorf("%d requests, want none", n)
	}
}

func TestJevJudgeCanceled(t *testing.T) {
	api := newFakeJev(t, answerWith("1"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := api.judge().Decide(ctx, questions("a")); err == nil {
		t.Error("want an error from a canceled context")
	}
}

// consult is what stands between a JevJudge and the analysis.
func TestJevJudgeAnswerOutOfRange(t *testing.T) {
	api := newFakeJev(t, answerWith("1.5"))

	if _, err := consult(context.Background(), api.judge(), questions("a")); err == nil {
		t.Error("want an error for a probability of 1.5")
	}
}

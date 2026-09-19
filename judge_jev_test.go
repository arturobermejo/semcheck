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

// fakeJev is an API that answers every request with the probability that
// answer gives to its state, and keeps the last request.
type fakeJev struct {
	*httptest.Server

	requests atomic.Int32
	body     map[string]any
}

func newFakeJev(t *testing.T, answer func(code string) string) *fakeJev {
	t.Helper()

	api := &fakeJev{}

	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.requests.Add(1)

		api.body = nil
		if err := json.NewDecoder(r.Body).Decode(&api.body); err != nil {
			t.Errorf("the request is not JSON: %v", err)
		}

		code := api.body["state"].(map[string]any)["code"].(string)
		io.WriteString(w, answer(code))
	}))
	t.Cleanup(api.Close)

	return api
}

func (api *fakeJev) judge() *JevJudge {
	return &JevJudge{APIKey: testKey, URL: api.URL}
}

func noul(p string) string {
	return `{"model": "jev-1.13.0", "answers": {"answer": {"type": "noul", "noul": ` + p + `}}}`
}

func TestJevJudgeRequest(t *testing.T) {
	api := newFakeJev(t, func(string) string { return noul("0.97") })

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
			"answer": map[string]any{"type": "noul", "instructions": "Does this log include personal data?"},
		},
	}

	got, _ := json.Marshal(api.body)
	if wantJSON, _ := json.Marshal(want); string(got) != string(wantJSON) {
		t.Errorf("body:\n got %s\nwant %s", got, wantJSON)
	}
}

func TestJevJudgeWithoutTypeNotes(t *testing.T) {
	api := newFakeJev(t, func(string) string { return noul("0") })

	decisions, err := api.judge().Decide(context.Background(), questions("x := 1"))
	if err != nil || decisions[0].Yes != 0 {
		t.Fatalf("Decide = %+v, %v; want a certain no", decisions, err)
	}

	state := api.body["state"].(map[string]any)
	if _, ok := state["variable_types"]; ok {
		t.Errorf("state = %v, want no variable_types", state)
	}
}

func TestJevJudgeModel(t *testing.T) {
	api := newFakeJev(t, func(string) string { return noul("0.5") })

	judge := api.judge()
	judge.Model = "jev-1.13.0"

	if _, err := judge.Decide(context.Background(), questions("a")); err != nil {
		t.Fatal(err)
	}

	if api.body["model"] != "jev-1.13.0" {
		t.Errorf("model = %v", api.body["model"])
	}
}

func TestJevJudgeKeepsTheOrder(t *testing.T) {
	api := newFakeJev(t, func(code string) string {
		return noul(map[string]string{"a": "0.1", "b": "0.2", "c": "0.3"}[code])
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

func TestJevJudgeStopsAtTheFirstFailure(t *testing.T) {
	answers := map[string]string{
		"not json":  "<html>",
		"no answer": `{"answers": {}}`,
		"a choice":  `{"answers": {"answer": {"type": "choice", "choice": "yes"}}}`,
	}

	for name, answer := range answers {
		t.Run(name, func(t *testing.T) {
			api := newFakeJev(t, func(code string) string {
				if code == "b" {
					return answer
				}

				return noul("1")
			})

			decisions, err := api.judge().Decide(context.Background(), questions("a", "b", "c"))
			if err == nil {
				t.Fatalf("decisions = %+v, want an error", decisions)
			}

			if !strings.Contains(err.Error(), "question 2 of 3 (rule no-pii-in-logs): jev: ") {
				t.Errorf("error %q does not say which question failed", err)
			}

			if n := api.requests.Load(); n != 2 {
				t.Errorf("%d requests, want it to stop at the failure", n)
			}
		})
	}
}

func TestJevJudgeWithoutKey(t *testing.T) {
	api := newFakeJev(t, func(string) string { return noul("1") })

	_, err := (&JevJudge{URL: api.URL}).Decide(context.Background(), questions("a"))
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("error = %v, want one about the API key", err)
	}

	if n := api.requests.Load(); n != 0 {
		t.Errorf("%d requests, want none", n)
	}
}

// consult is what stands between a JevJudge and the analysis.
func TestJevJudgeAnswerOutOfRange(t *testing.T) {
	api := newFakeJev(t, func(string) string { return noul("1.5") })

	if _, err := consult(context.Background(), api.judge(), questions("a")); err == nil {
		t.Error("want an error for a probability of 1.5")
	}
}

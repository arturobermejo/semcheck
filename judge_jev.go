package semcheck

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/arturobermejo/semcheck/internal/jev"
)

// APIKeyEnv is the variable DefaultJudge reads the key of JevJudge from, the
// same one the SDKs of TypeSafe use.
const APIKeyEnv = "TYPESAFE_API_KEY"

// DefaultJevConcurrency is how many requests a JevJudge has in flight at most.
const DefaultJevConcurrency = 8

// JevJudge asks Jev, the decision model of TypeSafe AI. It sends the code of
// every question, and the notes on its types, to the API: using it is a
// decision to share that code with a third party.
//
// A JevJudge must not be copied after its first use.
type JevJudge struct {
	APIKey string

	// URL, Model and Client default to the public API, its latest model and a
	// client with a timeout.
	URL    string
	Model  string
	Client *http.Client

	// MaxRetries is how many times a request that failed for a passing reason,
	// such as an overloaded API, is sent again: 2 if zero, never if negative.
	MaxRetries int

	// Concurrency is the most requests in flight, DefaultJevConcurrency if
	// zero. All the calls to Decide share it: drivers analyze packages in
	// parallel, and a limit for each would be no limit.
	Concurrency int

	once  sync.Once
	slots chan struct{}
}

var _ Judge = (*JevJudge)(nil)

// jevPrompt changes when what Jev gets to read for a question does: the names
// or the contents of jevState, or how the questions are put. The answers given
// to the old prompt say nothing about the new one.
const jevPrompt = "1"

// identity names what the answers of j depend on besides the questions, for
// the keys of the cache. With a model alias such as jev-latest, the answers of
// the version it meant before stay in use when it moves on.
func (j *JevJudge) identity() string {
	return "jev " + jevPrompt + " " + cmp.Or(j.Model, jev.DefaultModel)
}

// jevState is what Jev reads to answer. Its field names are part of the
// prompt: the model sees them.
type jevState struct {
	Code  string   `json:"code"`
	Types []string `json:"variable_types,omitempty"`
}

// A jevRequest has the questions that are about the same code, which is how the
// API takes them: one state and many questions. Two rules that look at the
// same log call cost one request.
type jevRequest struct {
	state   jevState
	indexes []int // of its questions, in the batch
}

func jevRequests(questions []Question) []*jevRequest {
	var (
		requests []*jevRequest
		byState  = map[string]*jevRequest{}
	)

	for i, q := range questions {
		key := strings.Join(append([]string{q.Fragment}, q.Types...), "\x00")

		r, ok := byState[key]
		if !ok {
			r = &jevRequest{state: jevState{q.Fragment, q.Types}}
			byState[key] = r
			requests = append(requests, r)
		}

		r.indexes = append(r.indexes, i)
	}

	return requests
}

// Decide sends the requests in parallel. The first failure stops it: the
// requests after it would most likely fail the same way. The answers it got by
// then are kept, and every other question gets the failure as its Err.
func (j *JevJudge) Decide(ctx context.Context, questions []Question) ([]Decision, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	var (
		client    = &jev.Client{APIKey: j.APIKey, URL: j.URL, Model: j.Model, HTTPClient: j.Client, MaxRetries: j.MaxRetries}
		decisions = make([]Decision, len(questions))
		answered  = make([]bool, len(questions))
		wg        sync.WaitGroup
	)

	for _, r := range jevRequests(questions) {
		if !j.acquire(ctx) {
			break
		}

		wg.Go(func() {
			defer j.release()

			if err := r.send(ctx, client, questions, decisions, answered); err != nil {
				cancel(err)
			}
		})
	}

	wg.Wait()

	if cause := context.Cause(ctx); cause != nil {
		for i := range decisions {
			if !answered[i] {
				decisions[i] = Decision{Err: cause}
			}
		}
	}

	return decisions, nil
}

// send writes the answers of r in its places of decisions and answered, which
// no other request touches.
func (r *jevRequest) send(ctx context.Context, client *jev.Client, questions []Question, decisions []Decision, answered []bool) error {
	asks := make(map[string]jev.Question, len(r.indexes))
	for n, i := range r.indexes {
		asks[fmt.Sprintf("q%d", n+1)] = jev.Noul(questions[i].Ask)
	}

	response, err := client.Ask(ctx, r.state, asks)
	if err != nil {
		return err
	}

	for n, i := range r.indexes {
		yes, err := response.Noul(fmt.Sprintf("q%d", n+1))
		if err != nil {
			return err
		}

		decisions[i].Yes, answered[i] = yes, true
	}

	return nil
}

// acquire waits for a free slot. It reports false if ctx ends first.
func (j *JevJudge) acquire(ctx context.Context) bool {
	j.once.Do(func() {
		j.slots = make(chan struct{}, cmp.Or(max(j.Concurrency, 0), DefaultJevConcurrency))
	})

	select {
	case j.slots <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (j *JevJudge) release() { <-j.slots }

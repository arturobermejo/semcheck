package semcheck

import (
	"context"
	"slices"
	"sync"
)

// FakeJudge is a Judge for tests: it answers with a function instead of a
// model, and remembers what it was asked. It is safe for concurrent use, since
// drivers analyze packages in parallel.
type FakeJudge struct {
	// Answer gives the probability of yes for a question. If nil, every answer
	// is a certain no.
	Answer func(Question) float64

	// Err, if set, makes Decide fail.
	Err error

	mu      sync.Mutex
	batches [][]Question
}

var _ Judge = (*FakeJudge)(nil)

func (f *FakeJudge) Decide(ctx context.Context, questions []Question) ([]Decision, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	f.mu.Lock()
	f.batches = append(f.batches, slices.Clone(questions))
	f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	decisions := make([]Decision, len(questions))

	if f.Answer != nil {
		for i, q := range questions {
			decisions[i].Yes = f.Answer(q)
		}
	}

	return decisions, nil
}

// Batches returns the questions of each call to Decide, in order.
func (f *FakeJudge) Batches() [][]Question {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.batches
}

// Questions returns every question asked so far, in order.
func (f *FakeJudge) Questions() []Question {
	return slices.Concat(f.Batches()...)
}

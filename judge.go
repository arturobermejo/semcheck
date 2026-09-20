package semcheck

import (
	"context"
	"fmt"
	"math"
)

// A Question is a closed question about a piece of code.
type Question struct {
	Rule     string   // the name of the rule that asks
	Ask      string   // the question, answerable with yes or no
	Fragment string   // the code it is about
	Types    []string // "name: type" notes on the variables the code uses
}

// A Decision is the answer to a Question.
type Decision struct {
	// Yes is the probability, from 0 to 1, that the answer is yes.
	Yes float64

	// Err, if set, says why this one question has no answer, when the judge
	// could answer the others of the batch. Yes is then meaningless.
	Err error

	// Cached says that nobody was asked: the decision was made in an earlier
	// run.
	Cached bool
}

// confidence is how sure the judge is that the answer is a.
func (d Decision) confidence(a Answer) float64 {
	if a == AnswerNo {
		return 1 - d.Yes
	}

	return d.Yes
}

// A Judge answers closed questions about code. The decision model behind
// semcheck is one; so is the FakeJudge used in tests.
//
// Decide takes a batch because a model answers many questions in one request
// far faster than in many. It returns one Decision per Question, in the same
// order, or an error if it could not answer.
type Judge interface {
	Decide(ctx context.Context, questions []Question) ([]Decision, error)
}

// consult asks the judge and checks what comes back. A Judge is code from
// outside, or a model behind a network: its answers are input, not facts.
func consult(ctx context.Context, judge Judge, questions []Question) ([]Decision, error) {
	if len(questions) == 0 {
		return nil, nil
	}

	decisions, err := judge.Decide(ctx, questions)
	if err != nil {
		return nil, fmt.Errorf("the judge failed: %w", err)
	}

	if len(decisions) != len(questions) {
		return nil, fmt.Errorf("the judge gave %d decisions for %d questions", len(decisions), len(questions))
	}

	for i, d := range decisions {
		if d.Err != nil {
			continue
		}

		if !validProbability(d.Yes) {
			return nil, fmt.Errorf("the judge gave the probability %v to question %d (rule %s)", d.Yes, i+1, questions[i].Rule)
		}
	}

	return decisions, nil
}

// validProbability checks for NaN by itself: it fails every comparison, so it
// would pass for a number that is neither under 0 nor over 1.
func validProbability(p float64) bool {
	return !math.IsNaN(p) && p >= 0 && p <= 1
}

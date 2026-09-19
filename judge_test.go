package semcheck

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"
)

func questions(fragments ...string) []Question {
	qs := make([]Question, len(fragments))
	for i, f := range fragments {
		qs[i] = Question{Rule: "no-pii-in-logs", Ask: "Does this log include personal data?", Fragment: f}
	}

	return qs
}

func TestFakeJudgeAnswers(t *testing.T) {
	pii := func(q Question) float64 {
		if strings.Contains(q.Fragment, "Email") {
			return 0.97
		}

		return 0.02
	}

	tests := []struct {
		name  string
		judge *FakeJudge
		want  []Decision
	}{
		{"without a function every answer is a certain no", &FakeJudge{}, []Decision{{0}, {0}, {0}}},
		{"one decision per question, in order", &FakeJudge{Answer: pii}, []Decision{{0.02}, {0.97}, {0.02}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.judge.Decide(context.Background(), questions("log.Print(id)", "log.Print(u.Email)", "log.Print(n)"))
			if err != nil {
				t.Fatal(err)
			}

			if !slices.Equal(got, tt.want) {
				t.Errorf("decisions = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFakeJudgeRemembers(t *testing.T) {
	judge := &FakeJudge{}
	first := questions("a", "b")

	if _, err := judge.Decide(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	if _, err := judge.Decide(context.Background(), questions("c")); err != nil {
		t.Fatal(err)
	}

	// What the caller does with its slice afterwards is not part of the record.
	first[0].Fragment = "changed"

	if got := judge.Batches(); len(got) != 2 || len(got[0]) != 2 || len(got[1]) != 1 {
		t.Fatalf("batches = %v, want one of 2 questions and one of 1", got)
	}

	var fragments []string
	for _, q := range judge.Questions() {
		fragments = append(fragments, q.Fragment)
	}

	if want := []string{"a", "b", "c"}; !slices.Equal(fragments, want) {
		t.Errorf("questions = %v, want %v", fragments, want)
	}
}

func TestFakeJudgeFails(t *testing.T) {
	boom := errors.New("rate limited")
	judge := &FakeJudge{Err: boom}

	decisions, err := judge.Decide(context.Background(), questions("a"))
	if !errors.Is(err, boom) || decisions != nil {
		t.Errorf("Decide = %v, %v; want nil and %v", decisions, err, boom)
	}

	if n := len(judge.Questions()); n != 1 {
		t.Errorf("a failed call must be on record too: got %d questions", n)
	}
}

func TestFakeJudgeHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	judge := &FakeJudge{}

	if _, err := judge.Decide(ctx, questions("a")); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}

	if n := len(judge.Questions()); n != 0 {
		t.Errorf("a canceled call reached the judge: %d questions on record", n)
	}
}

// Run with -race: drivers call the judge from several goroutines.
func TestFakeJudgeConcurrent(t *testing.T) {
	judge := &FakeJudge{Answer: func(Question) float64 { return 0.5 }}

	var wg sync.WaitGroup

	for range 50 {
		wg.Go(func() {
			if _, err := judge.Decide(context.Background(), questions("a", "b")); err != nil {
				t.Error(err)
			}

			judge.Questions()
		})
	}

	wg.Wait()

	if n := len(judge.Questions()); n != 100 {
		t.Errorf("got %d questions on record, want 100", n)
	}
}

// judgeFunc adapts a function to the Judge interface, for judges that misbehave.
type judgeFunc func([]Question) ([]Decision, error)

func (f judgeFunc) Decide(_ context.Context, qs []Question) ([]Decision, error) { return f(qs) }

func TestConsult(t *testing.T) {
	boom := errors.New("connection refused")

	answers := func(ps ...float64) Judge {
		return judgeFunc(func([]Question) ([]Decision, error) {
			ds := make([]Decision, len(ps))
			for i, p := range ps {
				ds[i].Yes = p
			}

			return ds, nil
		})
	}

	tests := []struct {
		name  string
		judge Judge
		want  string // part of the error; "" for success
	}{
		{"well behaved", answers(0, 1), ""},
		{"too few decisions", answers(0.5), "1 decisions for 2 questions"},
		{"too many decisions", answers(0.5, 0.5, 0.5), "3 decisions for 2 questions"},
		{"no decisions", answers(), "0 decisions for 2 questions"},
		{"negative probability", answers(0.5, -0.1), "the probability -0.1 to question 2 (rule no-pii-in-logs)"},
		{"probability above one", answers(1.5, 0.5), "the probability 1.5 to question 1"},
		{"a percentage", answers(0.5, 97), "the probability 97"},
		{"not a number", answers(math.NaN(), 0.5), "the probability NaN to question 1"},
		{"the judge fails", judgeFunc(func([]Question) ([]Decision, error) { return nil, boom }), "the judge failed: connection refused"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decisions, err := consult(context.Background(), tt.judge, questions("a", "b"))

			if tt.want == "" {
				if err != nil || len(decisions) != 2 {
					t.Errorf("consult = %v, %v", decisions, err)
				}

				return
			}

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tt.want)
			}

			if decisions != nil {
				t.Errorf("decisions came along with the error: %v", decisions)
			}
		})
	}

	if _, err := consult(context.Background(), judgeFunc(func([]Question) ([]Decision, error) { return nil, boom }), questions("a")); !errors.Is(err, boom) {
		t.Errorf("the cause got lost: %v", err)
	}
}

func TestConsultSkipsEmptyBatches(t *testing.T) {
	judge := &FakeJudge{}

	decisions, err := consult(context.Background(), judge, nil)
	if err != nil || decisions != nil {
		t.Errorf("consult = %v, %v", decisions, err)
	}

	if n := len(judge.Batches()); n != 0 {
		t.Errorf("the judge was called %d times for nothing", n)
	}
}

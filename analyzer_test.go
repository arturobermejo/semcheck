package semcheck

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

func TestAnalyzerValid(t *testing.T) {
	a := mustAnalyzer(t, &Config{Rules: []Rule{validRule()}}, &FakeJudge{})

	if err := analysis.Validate([]*analysis.Analyzer{Analyzer, a}); err != nil {
		t.Fatal(err)
	}
}

func mustAnalyzer(t *testing.T, cfg *Config, judge Judge) *analysis.Analyzer {
	t.Helper()

	a, err := NewAnalyzer(cfg, judge)
	if err != nil {
		t.Fatal(err)
	}

	return a
}

// fakeAnswers stands for the model in the end-to-end tests: it decides by what
// the fragment contains.
func fakeAnswers(q Question) float64 {
	has := func(s string) bool { return strings.Contains(q.Fragment, s) }

	switch q.Rule {
	case "no-pii-in-logs":
		switch {
		case has("Email"):
			return 0.97
		case has("fullName"):
			return 0.85
		}
	case "log-level-fits":
		if has("cannot reach") {
			return 0.88
		}
	case "doc-matches-code":
		// The probability that the comment is right.
		if has("func Save") {
			return 0.04
		}

		return 0.98
	case "test-name-matches":
		if has("func TestLogin") {
			return 0.93
		}
	}

	return 0.02
}

func TestAnalyzer(t *testing.T) {
	cfg, err := LoadConfig("testdata/config/rules.yml")
	if err != nil {
		t.Fatal(err)
	}

	judge := &FakeJudge{Answer: fakeAnswers}

	analysistest.Run(t, analysistest.TestData(), mustAnalyzer(t, cfg, judge), "rules")

	// The log inside the test file found nothing, but was it asked about?
	// Skipping has to happen before the question, not after the answer.
	for _, q := range judge.Questions() {
		if q.Rule == "no-pii-in-logs" && strings.Contains(q.Fragment, "created %s") {
			t.Errorf("no-pii-in-logs asked about a test file:\n%s", q.Fragment)
		}
	}
}

func TestAnalyzerRuleForTests(t *testing.T) {
	cfg, err := LoadConfig("testdata/config/rules.yml")
	if err != nil {
		t.Fatal(err)
	}

	cfg.Rules = cfg.Rules[:1]
	cfg.Rules[0].Tests = true

	analysistest.Run(t, analysistest.TestData(), mustAnalyzer(t, cfg, &FakeJudge{Answer: fakeAnswers}), "rulestests")
}

func TestNewAnalyzerRejectsInvalidConfig(t *testing.T) {
	bad := validRule()
	bad.MinConfidence = 7

	for _, cfg := range []*Config{{}, {Rules: []Rule{bad}}} {
		if a, err := NewAnalyzer(cfg, &FakeJudge{}); err == nil || a != nil {
			t.Errorf("NewAnalyzer(%+v) = %v, %v; want nil and an error", cfg, a, err)
		}
	}
}

const checkedSource = `package p

type User struct {
	ID    int
	Email string
}

func logf(format string, args ...any) {}

func create(u User) {
	logf("created %+v", u)
	logf("done")
}
`

// runOn type-checks src and runs the analyzer on it, without a driver.
func runOn(t *testing.T, a *analysis.Analyzer, src string) ([]analysis.Diagnostic, error) {
	t.Helper()

	_, pass := typeCheck(t, src, nil)
	pass.Analyzer = a
	pass.ResultOf = map[*analysis.Analyzer]any{inspect.Analyzer: inspector.New(pass.Files)}

	var diagnostics []analysis.Diagnostic

	pass.Report = func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }

	_, err := a.Run(pass)

	return diagnostics, err
}

func logRule() Rule {
	r := validRule()
	r.Match = MatchSpec{Matcher: "call", Args: []string{"p.logf"}}

	return r
}

func TestAnalyzerQuestions(t *testing.T) {
	judge := &FakeJudge{Answer: func(q Question) float64 {
		if strings.Contains(q.Fragment, "%+v") {
			return 0.95
		}

		return 0
	}}

	diagnostics, err := runOn(t, mustAnalyzer(t, &Config{Rules: []Rule{logRule()}}, judge), checkedSource)
	if err != nil {
		t.Fatal(err)
	}

	want := []Question{
		{
			Rule:     "no-pii-in-logs",
			Ask:      "Does this log include personal data?",
			Fragment: `logf("created %+v", u)`,
			Types:    []string{"u: User{ID int; Email string}"},
		},
		{
			Rule:     "no-pii-in-logs",
			Ask:      "Does this log include personal data?",
			Fragment: `logf("done")`,
		},
	}

	batches := judge.Batches()
	if len(batches) != 1 {
		t.Fatalf("the judge was called %d times, want once for the whole package", len(batches))
	}

	if !slices.EqualFunc(batches[0], want, func(a, b Question) bool {
		return a.Rule == b.Rule && a.Ask == b.Ask && a.Fragment == b.Fragment && slices.Equal(a.Types, b.Types)
	}) {
		t.Errorf("questions:\n%+v\nwant:\n%+v", batches[0], want)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("got %d diagnostics, want 1: %v", len(diagnostics), diagnostics)
	}

	d := diagnostics[0]
	if d.Category != "no-pii-in-logs" || d.Message != `no-pii-in-logs: the answer to "Does this log include personal data?" is yes (0.95)` {
		t.Errorf("diagnostic = %+v", d)
	}
}

func TestAnalyzerJudgeFails(t *testing.T) {
	boom := errors.New("rate limited")

	diagnostics, err := runOn(t, mustAnalyzer(t, &Config{Rules: []Rule{logRule()}}, &FakeJudge{Err: boom}), checkedSource)
	if !errors.Is(err, boom) {
		t.Errorf("error = %v, want one that wraps %v", err, boom)
	}

	if len(diagnostics) != 0 {
		t.Errorf("diagnostics came along with the error: %v", diagnostics)
	}
}

func TestAnalyzerNothingToAsk(t *testing.T) {
	judge := &FakeJudge{}

	if _, err := runOn(t, mustAnalyzer(t, &Config{Rules: []Rule{logRule()}}, judge), "package p\n\nfunc f() {}\n"); err != nil {
		t.Fatal(err)
	}

	if n := len(judge.Batches()); n != 0 {
		t.Errorf("the judge was called %d times for a package without matches", n)
	}
}

func TestAnalyzerSkipsHugeFragments(t *testing.T) {
	r := logRule()
	r.Context = ContextFunction

	src := "package p\n\nfunc logf(args ...any) {}\n\nfunc big(x int) {\n\tlogf(x)\n" + strings.Repeat("\tx = x + 100\n", maxFragmentBytes/13) + "}\n\nfunc small(x int) {\n\tlogf(x)\n}\n"

	judge := &FakeJudge{}
	if _, err := runOn(t, mustAnalyzer(t, &Config{Rules: []Rule{r}}, judge), src); err != nil {
		t.Fatal(err)
	}

	qs := judge.Questions()
	if len(qs) != 1 || !strings.HasPrefix(qs[0].Fragment, "func small") {
		t.Errorf("got %d questions, want only the one about small", len(qs))
	}
}

// deadlineJudge records the deadline it is given.
type deadlineJudge struct{ left time.Duration }

func (j *deadlineJudge) Decide(ctx context.Context, qs []Question) ([]Decision, error) {
	if deadline, ok := ctx.Deadline(); ok {
		j.left = time.Until(deadline)
	}

	return make([]Decision, len(qs)), nil
}

func TestAnalyzerBoundsTheJudge(t *testing.T) {
	judge := &deadlineJudge{}

	if _, err := runOn(t, mustAnalyzer(t, &Config{Rules: []Rule{logRule()}}, judge), checkedSource); err != nil {
		t.Fatal(err)
	}

	if judge.left <= 0 || judge.left > judgeTimeout {
		t.Errorf("the judge had %v left, want a deadline of at most %v", judge.left, judgeTimeout)
	}
}

func TestDecisionConfidence(t *testing.T) {
	tests := []struct {
		yes      float64
		reportIf Answer
		want     float64
	}{
		{0.97, AnswerYes, 0.97},
		{0.25, AnswerNo, 0.75},
		{0, AnswerNo, 1},
		{1, AnswerNo, 0},
		{0.5, AnswerYes, 0.5},
		{0.5, AnswerNo, 0.5},
	}

	for _, tt := range tests {
		if got := (Decision{Yes: tt.yes}).confidence(tt.reportIf); got != tt.want {
			t.Errorf("Decision{%v}.confidence(%s) = %v, want %v", tt.yes, tt.reportIf, got, tt.want)
		}
	}
}

func TestAnalyzerThreshold(t *testing.T) {
	tests := []struct {
		name     string
		yes      float64
		reportIf Answer
		min      float64
		reported bool
	}{
		{"above", 0.95, AnswerYes, 0.9, true},
		{"exactly at the threshold", 0.9, AnswerYes, 0.9, true},
		{"just below", 0.8999, AnswerYes, 0.9, false},
		{"a sure no, for a rule that reports yes", 0.02, AnswerYes, 0.9, false},
		{"a sure no, for a rule that reports no", 0.02, AnswerNo, 0.9, true},
		{"exactly at the threshold, reporting no", 0.1, AnswerNo, 0.9, true},
		{"just below, reporting no", 0.11, AnswerNo, 0.9, false},
		{"a sure yes, for a rule that reports no", 0.98, AnswerNo, 0.9, false},
		{"the threshold is per rule", 0.6, AnswerYes, 0.5, true},
		{"a threshold of one", 1, AnswerYes, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := logRule()
			r.ReportIf, r.MinConfidence = tt.reportIf, tt.min

			judge := &FakeJudge{Answer: func(Question) float64 { return tt.yes }}

			diagnostics, err := runOn(t, mustAnalyzer(t, &Config{Rules: []Rule{r}}, judge), "package p\n\nfunc logf(args ...any) {}\n\nfunc f() { logf(1) }\n")
			if err != nil {
				t.Fatal(err)
			}

			if got := len(diagnostics) == 1; got != tt.reported {
				t.Errorf("reported = %v, want %v (%v)", got, tt.reported, diagnostics)
			}
		})
	}
}

func TestAnalyzerSeveralRules(t *testing.T) {
	// Deliberately not in alphabetical order.
	second, first := logRule(), logRule()
	second.Name, first.Name = "b-rule", "a-rule"

	judge := &FakeJudge{Answer: func(Question) float64 { return 1 }}

	diagnostics, err := runOn(t, mustAnalyzer(t, &Config{Rules: []Rule{second, first}}, judge), checkedSource)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, d := range diagnostics {
		got = append(got, d.Category)
	}

	// By position, and by name where two rules meet.
	if want := []string{"a-rule", "b-rule", "a-rule", "b-rule"}; !slices.Equal(got, want) {
		t.Errorf("diagnostics = %v, want %v", got, want)
	}

	if n := len(judge.Batches()); n != 1 {
		t.Errorf("the judge was called %d times, want once for all the rules", n)
	}
}

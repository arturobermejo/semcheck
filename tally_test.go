package semcheck

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestTallyEstimate(t *testing.T) {
	var totals tally

	pii := Question{Rule: "no-pii-in-logs", Ask: strings.Repeat("a", 40), Fragment: strings.Repeat("f", 50), Types: []string{strings.Repeat("t", 9)}}
	name := Question{Rule: "name-matches-behavior", Ask: strings.Repeat("a", 40), Fragment: strings.Repeat("g", 360)}

	// 99 bytes are 50 tokens, and 400 are 200.
	got := totals.estimate([]Question{pii, name, pii})
	if want := "2 questions (name-matches-behavior 1, no-pii-in-logs 1), ~250 tokens; so far 2 questions, ~250 tokens, under $0.01"; got != want {
		t.Errorf("estimate = %q\nwant       %q", got, want)
	}

	// The variant of the package with its tests: the same two, and a new one.
	test := Question{Rule: "test-name-matches", Ask: "a", Fragment: strings.Repeat("h", 3)}

	got = totals.estimate([]Question{name, pii, test})
	if want := "1 question (test-name-matches 1), ~2 tokens; so far 3 questions, ~252 tokens, under $0.01"; got != want {
		t.Errorf("estimate = %q\nwant       %q", got, want)
	}

	if got = totals.estimate([]Question{name, test}); got != "" {
		t.Errorf("estimate = %q for questions that were all counted, want nothing", got)
	}
}

func TestDollars(t *testing.T) {
	for tokens, want := range map[int]string{
		0:           "under $0.01",
		200_000:     "under $0.01",
		800_000:     "~$0.03",
		100_000_000: "~$4.20",
	} {
		if got := dollars(tokens); got != want {
			t.Errorf("dollars(%d) = %q, want %q", tokens, got, want)
		}
	}
}

func TestTallyRecord(t *testing.T) {
	var totals tally

	asked := questions("a", "b", "c")

	got := totals.record(asked, []Decision{{Yes: 0.9}, {Yes: 0.1, Cached: true}, {Cached: true, Err: errFragmentTooLarge}}, &FakeJudge{})
	if want := "3 questions, 1 from the cache; so far 3 questions, 1 from the cache"; got != want {
		t.Errorf("record = %q\nwant     %q", got, want)
	}

	// Behind a cache, a judge that counts what it is billed. The estimate is
	// for what it was asked: the first question of each batch.
	judge := newCachedJudge(&JevJudge{}, &memoryCache{})
	judge.judge.(*JevJudge).billed.Store(7)

	got = totals.record(asked[:1], []Decision{{Yes: 0.9, Cached: true}}, judge)
	if want := fmt.Sprintf("1 question, 1 from the cache; so far 4 questions, 2 from the cache, ~%d tokens estimated, 7 billed", estimateTokens(asked[0])); got != want {
		t.Errorf("record = %q\nwant     %q", got, want)
	}

	// And a cache in front of a judge that does not count.
	got = totals.record(nil, nil, newCachedJudge(identifiedFake{&FakeJudge{}, "fake"}, &memoryCache{}))
	if strings.Contains(got, "billed") {
		t.Errorf("record = %q, with tokens nobody counted", got)
	}
}

// findingsIn returns how many findings a has in src.
func findingsIn(t *testing.T, a *analysis.Analyzer, src string) int {
	t.Helper()

	diagnostics, err := runOn(t, a, src)
	if err != nil {
		t.Fatal(err)
	}

	return len(diagnostics)
}

func TestDryRun(t *testing.T) {
	// Nobody to ask, and nothing needed to: not even a judge.
	a, out := testAnalyzer(t, &Config{Rules: []Rule{logRule()}, FailOnJudgeError: true}, nil, options{dryRun: true})

	if n := findingsIn(t, a, checkedSource); n != 0 {
		t.Errorf("%d findings in a dry run", n)
	}

	if n := findingsIn(t, a, "package p\n\nfunc logf(string) {}\n\nfunc f() { logf(\"other\") }\n"); n != 0 {
		t.Errorf("%d findings in a dry run", n)
	}

	// A package without questions has nothing to say.
	findingsIn(t, a, "package p\n")

	want := []string{
		"dry run: p: 2 questions (no-pii-in-logs 2), ~68 tokens; so far 2 questions, ~68 tokens, under $0.01",
		"dry run: p: 1 question (no-pii-in-logs 1), ~25 tokens; so far 3 questions, ~93 tokens, under $0.01",
	}
	if !slices.Equal(out.reports, want) {
		t.Errorf("reports = %q\nwant      %q", out.reports, want)
	}
}

func TestStats(t *testing.T) {
	fake := &FakeJudge{Answer: func(Question) float64 { return 0.95 }}
	judge := newCachedJudge(identifiedFake{fake, "fake 1"}, &memoryCache{})

	a, out := testAnalyzer(t, &Config{Rules: []Rule{logRule()}}, judge, options{stats: true})

	for range 2 {
		if n := findingsIn(t, a, checkedSource); n != 2 {
			t.Errorf("%d findings, want 2: stats change nothing", n)
		}
	}

	findingsIn(t, a, "package p\n")

	want := []string{
		"stats: p: 2 questions, 0 from the cache; so far 2 questions, 0 from the cache",
		"stats: p: 2 questions, 2 from the cache; so far 4 questions, 2 from the cache",
	}
	if !slices.Equal(out.reports, want) {
		t.Errorf("reports = %q\nwant      %q", out.reports, want)
	}
}

func TestNoStatsUnlessAskedFor(t *testing.T) {
	a, out := testAnalyzer(t, &Config{Rules: []Rule{logRule()}}, &FakeJudge{}, options{})

	findingsIn(t, a, checkedSource)

	if len(out.reports) != 0 {
		t.Errorf("reports = %q, want none", out.reports)
	}
}

func TestNewAnalyzerNeedsAJudge(t *testing.T) {
	if a, err := NewAnalyzer(&Config{Rules: []Rule{logRule()}}, nil); err == nil {
		t.Errorf("NewAnalyzer = %v without a judge, want an error", a)
	}
}

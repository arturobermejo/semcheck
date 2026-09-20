package semcheck

import (
	"slices"
	"strings"
	"testing"
)

func TestTallyEstimate(t *testing.T) {
	var totals tally

	pii := Question{Rule: "no-pii-in-logs", Ask: strings.Repeat("a", 40), Fragment: strings.Repeat("f", 50), Types: []string{strings.Repeat("t", 9)}}
	name := Question{Rule: "name-matches-behavior", Ask: strings.Repeat("a", 40), Fragment: strings.Repeat("g", 360)}

	// 99 bytes are 25 tokens, and 400 are 100.
	got := totals.estimate([]Question{pii, name, pii})
	if want := "2 questions (name-matches-behavior 1, no-pii-in-logs 1), ~125 tokens; so far 2 questions, ~125 tokens, under $0.01"; got != want {
		t.Errorf("estimate = %q\nwant       %q", got, want)
	}

	// The variant of the package with its tests: the same two, and a new one.
	test := Question{Rule: "test-name-matches", Ask: "a", Fragment: strings.Repeat("h", 3)}

	got = totals.estimate([]Question{name, pii, test})
	if want := "1 question (test-name-matches 1), ~1 tokens; so far 3 questions, ~126 tokens, under $0.01"; got != want {
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

	got := totals.record([]Decision{{Yes: 0.9}, {Yes: 0.1, Cached: true}, {Cached: true, Err: errFragmentTooLarge}})
	if want := "3 questions, 1 from the cache; so far 3 questions, 1 from the cache"; got != want {
		t.Errorf("record = %q\nwant     %q", got, want)
	}

	got = totals.record([]Decision{{Yes: 0.9, Cached: true}})
	if want := "1 question, 1 from the cache; so far 4 questions, 2 from the cache"; got != want {
		t.Errorf("record = %q\nwant     %q", got, want)
	}
}

func analyzerWithReports(t *testing.T, cfg *Config, judge Judge, opts options) (func(src string) int, *[]string) {
	t.Helper()

	var reports []string

	opts.report = func(msg string) { reports = append(reports, msg) }

	a, err := newAnalyzer(cfg, judge, opts)
	if err != nil {
		t.Fatal(err)
	}

	return func(src string) int {
		diagnostics, err := runOn(t, a, src)
		if err != nil {
			t.Fatal(err)
		}

		return len(diagnostics)
	}, &reports
}

func TestDryRun(t *testing.T) {
	// Nobody to ask, and nothing needed to: not even a judge.
	run, reports := analyzerWithReports(t, &Config{Rules: []Rule{logRule()}, FailOnJudgeError: true}, nil, options{dryRun: true})

	if n := run(checkedSource); n != 0 {
		t.Errorf("%d findings in a dry run", n)
	}

	if n := run("package p\n\nfunc logf(string) {}\n\nfunc f() { logf(\"other\") }\n"); n != 0 {
		t.Errorf("%d findings in a dry run", n)
	}

	// A package without questions has nothing to say.
	run("package p\n")

	want := []string{
		"dry run: p: 2 questions (no-pii-in-logs 2), ~34 tokens; so far 2 questions, ~34 tokens, under $0.01",
		"dry run: p: 1 question (no-pii-in-logs 1), ~13 tokens; so far 3 questions, ~47 tokens, under $0.01",
	}
	if !slices.Equal(*reports, want) {
		t.Errorf("reports = %q\nwant      %q", *reports, want)
	}
}

func TestStats(t *testing.T) {
	fake := &FakeJudge{Answer: func(Question) float64 { return 0.95 }}
	judge := newCachedJudge(identifiedFake{fake, "fake 1"}, &memoryCache{})

	run, reports := analyzerWithReports(t, &Config{Rules: []Rule{logRule()}}, judge, options{stats: true})

	for range 2 {
		if n := run(checkedSource); n != 2 {
			t.Errorf("%d findings, want 2: stats change nothing", n)
		}
	}

	run("package p\n")

	want := []string{
		"stats: p: 2 questions, 0 from the cache; so far 2 questions, 0 from the cache",
		"stats: p: 2 questions, 2 from the cache; so far 4 questions, 2 from the cache",
	}
	if !slices.Equal(*reports, want) {
		t.Errorf("reports = %q\nwant      %q", *reports, want)
	}
}

func TestNoStatsUnlessAskedFor(t *testing.T) {
	run, reports := analyzerWithReports(t, &Config{Rules: []Rule{logRule()}}, &FakeJudge{}, options{})

	run(checkedSource)

	if len(*reports) != 0 {
		t.Errorf("reports = %q, want none", *reports)
	}
}

func TestNewAnalyzerNeedsAJudge(t *testing.T) {
	if a, err := NewAnalyzer(&Config{Rules: []Rule{logRule()}}, nil); err == nil {
		t.Errorf("NewAnalyzer = %v without a judge, want an error", a)
	}
}

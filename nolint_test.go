package semcheck

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestSilencesSemcheck(t *testing.T) {
	tests := []struct {
		comment string
		want    bool
	}{
		{"//nolint", true},
		{"//nolint:semcheck", true},
		{"//nolint:all", true},
		{"//nolint:errcheck,semcheck", true},
		{"//nolint:errcheck, semcheck", true},
		{"//nolint:semcheck // the ID is not personal data", true},
		{"//nolint // everything here is fine", true},
		{"// nolint:semcheck", true},
		{"//nolint:SEMCHECK", true},
		{"//nolint:semcheck,", true},

		{"//nolint:errcheck", false},
		{"//nolint:errcheck // not semcheck", false},
		{"//nolint:semchecker", false},
		{"//nolinter", false},
		{"//nolint semcheck", false},
		{"// see nolint:semcheck in the docs", false},
		{"/* nolint:semcheck */", false},
		{"/*nolint*/", false},
		{"//", false},
		{"// a comment", false},
	}

	for _, tt := range tests {
		t.Run(tt.comment, func(t *testing.T) {
			if got := silencesSemcheck(tt.comment); got != tt.want {
				t.Errorf("silencesSemcheck(%q) = %v, want %v", tt.comment, got, tt.want)
			}
		})
	}
}

// Every call to logf is a finding unless a directive silences it. The letters
// and the expectations are the ones of cmd/semcheck/testdata/nolint, whose
// behavior under golangci-lint was observed, not assumed.
const nolintSource = `package p

func logf(args ...any) {}

func run(f func()) {}

func trailing() {
	logf("A")
	logf("B") //nolint:semcheck
	logf("C")
}

func ownLine() {
	//nolint:semcheck
	logf("D")
	logf("E")
}

//nolint:semcheck
func wholeFunction() {
	logf("F")
}

// docComment has the directive at the end of its doc comment.
//
//nolint:semcheck
func docComment() {
	logf("G")
}

func blocks(ok bool) {
	if ok { //nolint:semcheck
		logf("H")
	}

	logf("I",
		1, 2) //nolint:semcheck

	//nolint:semcheck
	if ok {
		logf("J")
	}
}

func edges() {
	//nolint:semcheck

	logf("U")

	//nolint:semcheck
	run(func() {
		logf("V")
	})

	/* nolint:semcheck */
	logf("W")

	//nolint:semcheck // the reason
	//
	// and more text in the same comment group
	logf("X")
}

// middleOfDoc is documented.
//nolint:semcheck
// The directive is in the middle of the doc comment.
func middleOfDoc() {
	logf("Y")
}
`

func TestAnalyzerSkipsSilencedCode(t *testing.T) {
	label := regexp.MustCompile(`logf\("([A-Z])"`)

	// The call to run contains V, which is silenced along with it.
	r := logRule()
	judge := &FakeJudge{Answer: func(Question) float64 { return 1 }}

	diagnostics, err := runOn(t, mustAnalyzer(t, &Config{Rules: []Rule{r}}, judge), nolintSource)
	if err != nil {
		t.Fatal(err)
	}

	var asked []string
	for _, q := range judge.Questions() {
		asked = append(asked, label.FindStringSubmatch(q.Fragment)[1])
	}

	// Not only unreported: never asked about.
	if want := strings.Split("ACEHIUW", ""); !slices.Equal(asked, want) {
		t.Errorf("asked about %v, want %v", asked, want)
	}

	if len(diagnostics) != len(asked) {
		t.Errorf("got %d diagnostics for %d questions", len(diagnostics), len(asked))
	}
}

func TestNolintAboveThePackageClause(t *testing.T) {
	const src = "//nolint:semcheck\npackage p\n\nfunc logf(args ...any) {}\n\nfunc f() {\n\tlogf(\"T\")\n}\n"

	judge := &FakeJudge{Answer: func(Question) float64 { return 1 }}

	if _, err := runOn(t, mustAnalyzer(t, &Config{Rules: []Rule{logRule()}}, judge), src); err != nil {
		t.Fatal(err)
	}

	if qs := judge.Questions(); len(qs) != 0 {
		t.Errorf("asked %d questions about a silenced file", len(qs))
	}
}

// Under golangci-lint the questions are asked all the same: see newAnalyzer.
func TestPluginLeavesNolintToGolangciLint(t *testing.T) {
	judge := &FakeJudge{Answer: func(Question) float64 { return 1 }}

	a, err := newAnalyzer(&Config{Rules: []Rule{logRule()}}, judge, false)
	if err != nil {
		t.Fatal(err)
	}

	diagnostics, err := runOn(t, a, nolintSource)
	if err != nil {
		t.Fatal(err)
	}

	if n := len(judge.Questions()); n != 15 || len(diagnostics) != 15 {
		t.Errorf("got %d questions and %d diagnostics, want all 15", n, len(diagnostics))
	}
}

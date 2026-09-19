package semcheck

import (
	"slices"
	"strings"
	"testing"
)

func validRule() Rule {
	return Rule{
		Name:          "no-pii-in-logs",
		Match:         MatchSpec{Matcher: "call", Args: []string{"log.*"}},
		Ask:           "Does this log include personal data?",
		ReportIf:      AnswerYes,
		MinConfidence: 0.9,
		Context:       ContextStatement,
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Rule)
		want   string // part of the error; "" if the rule is valid
	}{
		{"nothing wrong", func(r *Rule) {}, ""},
		{"every optional value", func(r *Rule) {
			r.ReportIf, r.MinConfidence, r.Context, r.Severity = AnswerNo, 1, ContextFunction, "warning"
		}, ""},
		{"name with digits, dashes and underscores", func(r *Rule) { r.Name = "No_PII-in-logs2" }, ""},

		{"no name", func(r *Rule) { r.Name = "" }, "rule #1: name is required"},
		{"name with a space", func(r *Rule) { r.Name = "no pii" }, `rule #1 "no pii": name must start with a letter`},
		{"name with a colon", func(r *Rule) { r.Name = "pii:logs" }, "name must start with a letter"},
		{"name that starts with a digit", func(r *Rule) { r.Name = "2fa" }, "name must start with a letter"},
		{"no question", func(r *Rule) { r.Ask = "" }, "ask is required"},
		{"blank question", func(r *Rule) { r.Ask = " \n\t" }, "ask is required"},
		{"report_if set from Go", func(r *Rule) { r.ReportIf = "maybe" }, `report_if must be yes or no, not "maybe"`},
		{"zero confidence", func(r *Rule) { r.MinConfidence = 0 }, "min_confidence must be greater than 0 and at most 1, not 0"},
		{"negative confidence", func(r *Rule) { r.MinConfidence = -0.5 }, "not -0.5"},
		{"confidence as a percentage", func(r *Rule) { r.MinConfidence = 90 }, "not 90"},
		{"unknown context", func(r *Rule) { r.Context = "file" }, `context must be statement or function, not "file"`},
		{"unknown severity", func(r *Rule) { r.Severity = "fatal" }, `severity must be error, warning or info, not "fatal"`},
		{"no matcher", func(r *Rule) { r.Match = MatchSpec{} }, "match is required"},
		{"unknown matcher", func(r *Rule) { r.Match = MatchSpec{Matcher: "http-handler"} }, `match: unknown matcher "http-handler" (the matchers are: call, exported-func-doc, func-prefix, test-func)`},
		{"bad arguments for the matcher", func(r *Rule) { r.Match.Args = []string{"log"} }, `match: call: "log" must look like pkg.Func`},
		{"arguments for a matcher without them", func(r *Rule) { r.Match = MatchSpec{"test-func", []string{"x"}}; r.Context = ContextFunction }, "match: test-func takes no arguments"},
		{"statement context on whole functions", func(r *Rule) { r.Match = MatchSpec{Matcher: "func-prefix", Args: []string{"Get"}} }, "context: statement has no effect with the matcher func-prefix"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validRule()
			tt.change(&r)

			err := (&Config{Rules: []Rule{r}}).Validate()

			switch {
			case tt.want == "" && err != nil:
				t.Errorf("Validate() = %v, want no error", err)
			case tt.want != "" && err == nil:
				t.Errorf("Validate() accepted the rule, want an error with %q", tt.want)
			case tt.want != "" && !strings.Contains(err.Error(), tt.want):
				t.Errorf("Validate() = %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestValidateNoRules(t *testing.T) {
	for _, cfg := range []*Config{{}, {Rules: []Rule{}}} {
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate() accepted %+v", cfg)
		}
	}
}

func TestValidateDuplicateNames(t *testing.T) {
	a, b, c := validRule(), validRule(), validRule()
	c.Name = "another"

	err := (&Config{Rules: []Rule{a, c, b}}).Validate()
	if err == nil || !strings.Contains(err.Error(), `rule #3 "no-pii-in-logs": there is another rule with that name`) {
		t.Fatalf("Validate() = %v", err)
	}

	if n := len(joined(err)); n != 1 {
		t.Errorf("got %d problems, want 1: the duplicate is reported once\n%v", n, err)
	}

	// Two rules without a name are not duplicates of each other.
	a.Name, b.Name = "", ""

	if n := len(joined((&Config{Rules: []Rule{a, b}}).Validate())); n != 2 {
		t.Errorf("got %d problems, want 2 missing names", n)
	}
}

// joined returns the errors inside one made with errors.Join.
func joined(err error) []error {
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		return multi.Unwrap()
	}

	return nil
}

func TestLoadConfigReportsEveryProblem(t *testing.T) {
	_, err := LoadConfig("testdata/config/invalid.yml")
	if err == nil {
		t.Fatal("LoadConfig accepted invalid.yml")
	}

	want := []string{
		"semcheck: testdata/config/invalid.yml: invalid configuration:",
		`rule #1 "no-pii-in-logs": min_confidence must be greater than 0 and at most 1, not 7`,
		`rule #1 "no-pii-in-logs": match: call: "log" must look like pkg.Func or pkg.*`,
		`rule #2 "no-pii-in-logs": there is another rule with that name`,
		`rule #2 "no-pii-in-logs": ask is required`,
		`rule #2 "no-pii-in-logs": match: unknown matcher "http-handler"`,
		`rule #3: name is required`,
		`rule #3: severity must be error, warning or info, not "fatal"`,
		`rule #3: context: statement has no effect with the matcher exported-func-doc`,
	}

	lines := strings.Split(err.Error(), "\n")

	for _, w := range want {
		if !slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, w) }) {
			t.Errorf("no line of the error starts with %q", w)
		}
	}

	if len(lines) != len(want) {
		t.Errorf("got %d lines, want %d:\n%s", len(lines), len(want), err)
	}
}

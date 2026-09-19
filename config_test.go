package semcheck

import (
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	got, err := LoadConfig("testdata/config/semcheck.yml")
	if err != nil {
		t.Fatal(err)
	}

	want := &Config{Rules: []Rule{
		{
			Name:          "no-pii-in-logs",
			Match:         MatchSpec{Matcher: "call", Args: []string{"log.*", "slog.*"}},
			Ask:           "Does this log include personal data (email, ID number, phone, full name, card)?",
			ReportIf:      AnswerYes,
			MinConfidence: 0.95,
			Context:       ContextStatement,
		},
		{
			Name:          "doc-matches-code",
			Match:         MatchSpec{Matcher: "exported-func-doc"},
			Ask:           "Does the comment describe what the function does?",
			ReportIf:      AnswerNo,
			MinConfidence: DefaultMinConfidence,
			Context:       DefaultContext,
		},
		{
			Name:          "name-matches-behavior",
			Match:         MatchSpec{Matcher: "func-prefix", Args: []string{"Get", "Is"}},
			Ask:           "Does this function modify state although its name suggests it only reads?",
			ReportIf:      DefaultReportIf,
			MinConfidence: DefaultMinConfidence,
			Context:       DefaultContext,
		},
	}}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("config:\n%+v\nwant:\n%+v", got, want)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	_, err := LoadConfig("testdata/config/does-not-exist.yml")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("error = %v, want one that wraps fs.ErrNotExist", err)
	}

	_, err = LoadConfig("testdata/config/broken.yml")
	if err == nil {
		t.Fatal("LoadConfig accepted broken.yml")
	}

	for _, want := range []string{"semcheck: ", "testdata/config/broken.yml", "line 4", "min_confidnce"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// rule wraps the fields of a single rule in a configuration.
func rule(fields string) string {
	return "rules:\n  - name: r\n    ask: q\n" + fields
}

func TestReportIf(t *testing.T) {
	tests := []struct {
		yaml string
		want Answer // "" for an error
	}{
		{"yes", AnswerYes},
		{"no", AnswerNo},
		{"true", AnswerYes},
		{"false", AnswerNo},
		{"Yes", AnswerYes},
		{"NO", AnswerNo},
		{"on", AnswerYes},
		{"off", AnswerNo},
		{`"yes"`, AnswerYes},

		{"maybe", ""},
		{"1", ""},
		{"[yes]", ""},
		{"{yes: no}", ""},
		{`""`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.yaml, func(t *testing.T) {
			cfg, err := parseConfig([]byte(rule("    match: call\n    report_if: " + tt.yaml + "\n")))

			if tt.want == "" {
				if err == nil || !strings.Contains(err.Error(), "line 5") {
					t.Fatalf("error = %v, want one at line 5", err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if got := cfg.Rules[0].ReportIf; got != tt.want {
				t.Errorf("report_if = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMatchSpec(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want MatchSpec
		err  bool
	}{
		{"flow mapping with a list", `{ call: ["log.*", "slog.*"] }`, MatchSpec{"call", []string{"log.*", "slog.*"}}, false},
		{"no arguments as an empty mapping", `{ test-func: {} }`, MatchSpec{Matcher: "test-func"}, false},
		{"no arguments as nothing", `{ test-func: }`, MatchSpec{Matcher: "test-func"}, false},
		{"no arguments as an empty list", `{ test-func: [] }`, MatchSpec{Matcher: "test-func"}, false},
		{"just the name", `test-func`, MatchSpec{Matcher: "test-func"}, false},
		{"unquoted arguments", `{ func-prefix: [Get, Is] }`, MatchSpec{"func-prefix", []string{"Get", "Is"}}, false},
		{"arguments that look like other types", `{ func-prefix: [yes, 1, null] }`, MatchSpec{"func-prefix", []string{"yes", "1", "null"}}, false},

		{"two matchers", `{ call: ["log.*"], test-func: {} }`, MatchSpec{}, true},
		{"no matcher", `{}`, MatchSpec{}, true},
		{"a list of matchers", `[call]`, MatchSpec{}, true},
		{"empty name", `""`, MatchSpec{}, true},
		{"a single argument outside a list", `{ call: "log.*" }`, MatchSpec{}, true},
		{"named arguments", `{ call: { patterns: ["log.*"] } }`, MatchSpec{}, true},
		{"nested lists", `{ call: [["log.*"]] }`, MatchSpec{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseConfig([]byte(rule("    match: " + tt.yaml + "\n")))

			if tt.err {
				if err == nil || !strings.Contains(err.Error(), "line 4") {
					t.Fatalf("error = %v, want one at line 4", err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if got := cfg.Rules[0].Match; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("match = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseConfigRejects(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string // part of the error
	}{
		{"an empty file", "", "empty"},
		{"only comments", "# nothing here\n", "empty"},
		{"two documents", "rules: []\n---\nrules: []\n", "more than one"},
		{"an unknown top-level field", "rule: []\n", "line 1: field rule not found"},
		{"an unknown field of a rule", rule("    min_confidnce: 0.5\n"), "line 4: field min_confidnce not found"},
		{"a repeated field", rule("    ask: again\n"), "line 4"},
		{"a field of the wrong type", rule("    min_confidence: high\n"), "line 4"},
		{"rules that are not a list", "rules:\n  name: r\n", "line 2"},
		{"syntax that is not YAML", "rules: [\n", "line"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseConfig([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("parseConfig accepted it: %+v", cfg)
			}

			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := parseConfig([]byte(rule("    match: call\n")))
	if err != nil {
		t.Fatal(err)
	}

	r := cfg.Rules[0]
	if r.ReportIf != AnswerYes || r.MinConfidence != 0.9 || r.Context != ContextFunction {
		t.Errorf("defaults = %+v", r)
	}

	// No rules at all is a well-formed file: whether it makes sense is not
	// decided here.
	for _, src := range []string{"rules: []\n", "rules:\n", "{}\n"} {
		cfg, err := parseConfig([]byte(src))
		if err != nil || len(cfg.Rules) != 0 {
			t.Errorf("parseConfig(%q) = %+v, %v", src, cfg, err)
		}
	}
}

func TestParseConfigFailOnJudgeError(t *testing.T) {
	for src, want := range map[string]bool{
		rule("    match: call\n"):                                  false,
		"fail_on_judge_error: true\n" + rule("    match: call\n"):  true,
		"fail_on_judge_error: false\n" + rule("    match: call\n"): false,
	} {
		cfg, err := parseConfig([]byte(src))
		if err != nil {
			t.Fatal(err)
		}

		if cfg.FailOnJudgeError != want {
			t.Errorf("fail_on_judge_error = %v, want %v for:\n%s", cfg.FailOnJudgeError, want, src)
		}
	}
}

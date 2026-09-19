package semcheck

import (
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func TestPluginRegistered(t *testing.T) {
	newPlugin, err := register.GetPlugin(PluginName)
	if err != nil {
		t.Fatal(err)
	}

	p, err := newPlugin(map[string]any{"config": "testdata/config/rules.yml"})
	if err != nil {
		t.Fatal(err)
	}

	analyzers, err := p.BuildAnalyzers()
	if err != nil {
		t.Fatal(err)
	}

	if len(analyzers) != 1 || analyzers[0].Name != PluginName {
		t.Fatalf("analyzers = %v, want one named %q", analyzers, PluginName)
	}

	if err := analysis.Validate(analyzers); err != nil {
		t.Error(err)
	}

	if got := p.GetLoadMode(); got != register.LoadModeTypesInfo {
		t.Errorf("load mode = %q, want %q", got, register.LoadModeTypesInfo)
	}
}

func TestPluginConfig(t *testing.T) {
	// The settings as golangci-lint decodes them from its own YAML.
	inline := map[string]any{
		"rules": []any{
			map[string]any{
				"name":      "no-pii-in-logs",
				"match":     map[string]any{"call": []any{"log.*"}},
				"ask":       "Does this log include personal data?",
				"report_if": true,
			},
			map[string]any{
				"name":  "doc-matches-code",
				"match": "exported-func-doc",
				"ask":   "Does the comment describe what the function does?",
			},
		},
	}

	tests := []struct {
		name     string
		settings any
		dir      string // where golangci-lint runs; "" for the root of the repository
		want     []string
		err      string
	}{
		{name: "a file", settings: map[string]any{"config": "testdata/config/project/.semcheck.yml"}, want: []string{"found-upwards"}},
		{name: "rules in the settings", settings: inline, want: []string{"no-pii-in-logs", "doc-matches-code"}},
		{name: "no settings, a file in the directory", dir: "testdata/config/project", want: []string{"found-upwards"}},
		{name: "no settings, a file in a parent", dir: "testdata/config/project/internal/pkg", want: []string{"found-upwards"}},
		{name: "empty settings", settings: map[string]any{}, dir: "testdata/config/project", want: []string{"found-upwards"}},

		{name: "no settings and no file", err: "no .semcheck.yml in"},
		{name: "both", settings: map[string]any{"config": "x.yml", "rules": inline["rules"]}, err: "either config or rules"},
		{name: "a misspelled setting", settings: map[string]any{"confg": "x.yml"}, err: "field confg not found"},
		{name: "a misspelled field of a rule", settings: map[string]any{"rules": []any{map[string]any{"name": "r", "asks": "q"}}}, err: "field asks not found"},
		{name: "a file that does not exist", settings: map[string]any{"config": "nope.yml"}, err: "nope.yml"},
		{name: "a file with invalid rules", settings: map[string]any{"config": "testdata/config/invalid.yml"}, err: "invalid configuration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.dir != "" {
				t.Chdir(tt.dir)
			}

			cfg, err := pluginConfig(tt.settings)

			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("error = %v, want it to mention %q", err, tt.err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			var names []string
			for _, r := range cfg.Rules {
				names = append(names, r.Name)
			}

			if !slices.Equal(names, tt.want) {
				t.Errorf("rules = %v, want %v", names, tt.want)
			}
		})
	}
}

func TestPluginInlineRules(t *testing.T) {
	cfg, err := pluginConfig(map[string]any{"rules": []any{map[string]any{
		"name":      "doc-matches-code",
		"match":     map[string]any{"func-prefix": []any{"Get", "Is"}},
		"ask":       "q",
		"report_if": false,
	}}})
	if err != nil {
		t.Fatal(err)
	}

	// Decoded like a file: booleans as answers, arguments as strings, defaults.
	want := Rule{
		Name:          "doc-matches-code",
		Match:         MatchSpec{"func-prefix", []string{"Get", "Is"}},
		Ask:           "q",
		ReportIf:      AnswerNo,
		MinConfidence: DefaultMinConfidence,
		Context:       DefaultContext,
	}

	if got := cfg.Rules[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("rule = %+v, want %+v", got, want)
	}
}

func TestPluginRejectsInvalidInlineRules(t *testing.T) {
	_, err := newPlugin(map[string]any{"rules": []any{map[string]any{"name": "r", "match": "http-handler", "ask": "q"}}})
	if err == nil || !strings.Contains(err.Error(), `unknown matcher "http-handler"`) {
		t.Errorf("error = %v", err)
	}
}

func TestGolangciLintVersionsMatch(t *testing.T) {
	versionIn := func(file, pattern string) string {
		t.Helper()

		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}

		m := regexp.MustCompile(pattern).FindSubmatch(data)
		if m == nil {
			t.Fatalf("%s: no match for %s", file, pattern)
		}

		return string(m[1])
	}

	makefile := versionIn("Makefile", `(?m)^GOLANGCI_LINT_VERSION := (\S+)$`)
	custom := versionIn(".custom-gcl.yml", `(?m)^version: (\S+)$`)

	if makefile != custom {
		t.Errorf("Makefile installs golangci-lint %s, .custom-gcl.yml builds against %s", makefile, custom)
	}
}

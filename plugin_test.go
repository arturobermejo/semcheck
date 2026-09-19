package semcheck

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

// TestPluginRegistered goes through the registry, the same way golangci-lint
// does: it knows the plugin by name only.
func TestPluginRegistered(t *testing.T) {
	newPlugin, err := register.GetPlugin(PluginName)
	if err != nil {
		t.Fatal(err)
	}

	p, err := newPlugin(nil)
	if err != nil {
		t.Fatal(err)
	}

	analyzers, err := p.BuildAnalyzers()
	if err != nil {
		t.Fatal(err)
	}

	if want := []*analysis.Analyzer{Analyzer}; !slices.Equal(analyzers, want) {
		t.Errorf("analyzers = %v, want %v", analyzers, want)
	}

	if err := analysis.Validate(analyzers); err != nil {
		t.Error(err)
	}

	if got := p.GetLoadMode(); got != register.LoadModeTypesInfo {
		t.Errorf("load mode = %q, want %q", got, register.LoadModeTypesInfo)
	}
}

// TestGolangciLintVersionsMatch guards a duplication that cannot be removed:
// the plugin is compiled against one exact golangci-lint version, and the
// Makefile installs the binary that performs that build.
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

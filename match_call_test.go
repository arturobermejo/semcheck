package semcheck

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

func TestCallTo(t *testing.T) {
	m, err := callTo("log.*", "slog.*", "zap.*", "zerolog.*")
	if err != nil {
		t.Fatal(err)
	}

	analysistest.Run(t, analysistest.TestData(), newMatchAnalyzer(m), "calls")
}

func TestCallToImportPaths(t *testing.T) {
	m, err := callTo("github.com/rs/zerolog/log.*", "log/slog.Info")
	if err != nil {
		t.Fatal(err)
	}

	analysistest.Run(t, analysistest.TestData(), newMatchAnalyzer(m), "callpaths")
}

func TestCallToInvalid(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
	}{
		{"no patterns", nil},
		{"empty", []string{""}},
		{"no function", []string{"log"}},
		{"trailing dot", []string{"log."}},
		{"no package", []string{".Printf"}},
		{"import path without function", []string{"go.uber.org/zap"}},
		{"bad glob", []string{"log.[Printf"}},
		{"second pattern is bad", []string{"log.*", "slog"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if m, err := callTo(tt.patterns...); err == nil || m != nil {
				t.Errorf("callTo(%q) = %v, %v; want nil and an error", tt.patterns, m, err)
			}
		})
	}
}

func TestParseCallPattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    callPattern
	}{
		{"log.*", callPattern{"log", "*"}},
		{"log.Printf", callPattern{"log", "Printf"}},
		{"log.Print*", callPattern{"log", "Print*"}},
		{"log/slog.Info", callPattern{"log/slog", "Info"}},
		{"go.uber.org/zap.*", callPattern{"go.uber.org/zap", "*"}},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got, err := parseCallPattern(tt.pattern)
			if err != nil {
				t.Fatal(err)
			}

			if got != tt.want {
				t.Errorf("parseCallPattern(%q) = %+v, want %+v", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestCallPatternMatches(t *testing.T) {
	zlog := types.NewPackage("github.com/rs/zerolog/log", "log")
	fn := func(pkg *types.Package, name string) *types.Func {
		return types.NewFunc(token.NoPos, pkg, name, types.NewSignatureType(nil, nil, nil, nil, nil, false))
	}

	tests := []struct {
		pattern string
		fn      *types.Func
		want    bool
	}{
		{"github.com/rs/zerolog/log.*", fn(zlog, "Info"), true},
		{"log.*", fn(zlog, "Info"), true},
		{"log.Info", fn(zlog, "Info"), true},
		{"log.In*", fn(zlog, "Info"), true},

		{"log.Error", fn(zlog, "Info"), false},
		{"log.info", fn(zlog, "Info"), false},
		{"zerolog.*", fn(zlog, "Info"), false},
		{"rs/zerolog/log.*", fn(zlog, "Info"), false},

		// The Error method of the predeclared error type has no package.
		{"log.*", fn(nil, "Error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			p, err := parseCallPattern(tt.pattern)
			if err != nil {
				t.Fatal(err)
			}

			if got := p.matches(tt.fn); got != tt.want {
				t.Errorf("%q matches %v = %v, want %v", tt.pattern, tt.fn, got, tt.want)
			}
		})
	}
}

// describe spells out the function and the statement around a match, which
// the regular match analyzer does not report.
func describe(pass *analysis.Pass, m match) string {
	var where string

	switch fn := m.Func.(type) {
	case nil:
		where = "package level"
	case *ast.FuncDecl:
		where = "in " + fn.Name.Name
	default:
		where = fmt.Sprintf("in the literal of line %d", pass.Fset.Position(fn.Pos()).Line)
	}

	if m.Stmt == nil {
		return where + ", no statement"
	}

	return fmt.Sprintf("%s, %T", where, m.Stmt)
}

func TestMatchSurroundings(t *testing.T) {
	m := must(callTo("log.*"))

	a := &analysis.Analyzer{
		Name:     "enclosing",
		Doc:      "reports the function and the statement around each match",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			for _, m := range []*matcher{m, exportedFuncDoc} {
				for _, hit := range m.matches(pass) {
					pass.Reportf(hit.Pos, "%s", describe(pass, hit))
				}
			}

			return nil, nil
		},
	}

	analysistest.Run(t, analysistest.TestData(), a, "enclosing")
}

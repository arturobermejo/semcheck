package semcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// runOnSource is a toy driver: it parses one source file, builds the
// analysis.Pass by hand and runs Analyzer on it. Real drivers (singlechecker,
// golangci-lint, analysistest) do the same job for whole packages.
func runOnSource(t *testing.T, src string) []string {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "example.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var got []string

	pass := &analysis.Pass{
		Analyzer: Analyzer,
		Fset:     fset,
		Files:    []*ast.File{file},
		Report: func(d analysis.Diagnostic) {
			pos := fset.Position(d.Pos)
			got = append(got, fmt.Sprintf("%d:%d: %s", pos.Line, pos.Column, d.Message))
		},
	}

	result, err := Analyzer.Run(pass)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if result != nil {
		t.Fatalf("result = %v, want nil", result)
	}

	return got
}

func TestAnalyzerValid(t *testing.T) {
	if err := analysis.Validate([]*analysis.Analyzer{Analyzer}); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyzerReportsFuncDecls(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "no declarations",
			src:  "package p\n",
			want: nil,
		},
		{
			name: "one function",
			src: `package p

func Hello() {}
`,
			want: []string{"3:6: found function Hello"},
		},
		{
			name: "position is the name, not the func keyword",
			src: `package p

	func    indented() {}
`,
			want: []string{"3:10: found function indented"},
		},
		{
			name: "several functions in source order",
			src: `package p

func a() {}

func b() {}
`,
			want: []string{
				"3:6: found function a",
				"5:6: found function b",
			},
		},
		{
			name: "methods are function declarations too",
			src: `package p

type T struct{}

func (t T) Method() {}
`,
			want: []string{"5:12: found function Method"},
		},
		{
			name: "other declarations are ignored",
			src: `package p

import "fmt"

const c = 1

var v = fmt.Sprint(c)

type T int
`,
			want: nil,
		},
		{
			name: "function literals are expressions, not declarations",
			src: `package p

var f = func() {}

func outer() {
	inner := func() {}
	inner()
}
`,
			want: []string{"5:6: found function outer"},
		},
		{
			name: "declaration without body",
			src: `package p

func implementedInAssembly(x int) int
`,
			want: []string{"3:6: found function implementedInAssembly"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runOnSource(t, tt.src)
			if !slices.Equal(got, tt.want) {
				t.Errorf("diagnostics:\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

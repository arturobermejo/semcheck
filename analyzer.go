package semcheck

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// Analyzer is the semcheck analysis.
//
// For now it is a laboratory analyzer: it reports every function and method
// declared in the package, to show how go/analysis hands us the syntax trees
// and how diagnostics are reported.
var Analyzer = &analysis.Analyzer{
	Name: "semcheck",
	Doc:  "reports every function declaration (laboratory analyzer)",
	Run:  run,
}

// run is called once per package. It must not keep state between calls:
// drivers may analyze packages in parallel or in separate processes.
func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		// Nobody can act on findings in generated code, such as the main
		// package that "go test" synthesizes for every tested package.
		if ast.IsGenerated(file) {
			continue
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}

			pass.Reportf(fn.Name.Pos(), "found function %s", fn.Name.Name)
		}
	}

	return nil, nil
}

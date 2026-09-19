package semcheck

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the semcheck analysis.
//
// For now it is a laboratory analyzer: it reports every function declaration
// and every function literal in the package, at any depth, to show how
// go/analysis hands us the syntax trees and how to traverse them.
var Analyzer = &analysis.Analyzer{
	Name:     "semcheck",
	Doc:      "reports every function declaration and literal (laboratory analyzer)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// run is called once per package. It must not keep state between calls:
// drivers may analyze packages in parallel or in separate processes.
func run(pass *analysis.Pass) (any, error) {
	in := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// The children of the root are the files of the package.
	for fileCur := range in.Root().Children() {
		// Nobody can act on findings in generated code, such as the main
		// package that "go test" synthesizes for every tested package.
		if ast.IsGenerated(fileCur.Node().(*ast.File)) {
			continue
		}

		for cur := range fileCur.Preorder((*ast.FuncDecl)(nil), (*ast.FuncLit)(nil)) {
			switch n := cur.Node().(type) {
			case *ast.FuncDecl:
				pass.Reportf(n.Name.Pos(), "found function %s", n.Name.Name)
			case *ast.FuncLit:
				pass.Reportf(n.Pos(), "found function literal")
			}
		}
	}

	return nil, nil
}

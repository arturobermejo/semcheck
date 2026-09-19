package semcheck

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the semcheck analysis. For now it is a laboratory analyzer that
// reports every function declaration and literal.
var Analyzer = &analysis.Analyzer{
	Name:     "semcheck",
	Doc:      "reports every function declaration and literal (laboratory analyzer)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	in := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	for fileCur := range in.Root().Children() {
		// Includes the main package that "go test" synthesizes.
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

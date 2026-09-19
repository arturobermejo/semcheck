package semcheck

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// testFunc selects the test functions: TestXxx(*testing.T) in _test.go files.
var testFunc = &Matcher{
	Name:     "test-func",
	Types:    []ast.Node{(*ast.FuncDecl)(nil)},
	ForTests: true,
	Match: func(pass *analysis.Pass, cur inspector.Cursor) (Match, bool) {
		fn := cur.Node().(*ast.FuncDecl)

		if fn.Body == nil || !isTestName(fn.Name.Name) || !inTestFile(pass, fn) || !hasTestSignature(pass, fn) {
			return Match{}, false
		}

		return Match{Node: fn, Pos: fn.Name.Pos()}, true
	},
}

// isTestName follows "go test": Test, TestLogin and Test_login are tests,
// Testify is not.
func isTestName(name string) bool {
	rest, ok := strings.CutPrefix(name, "Test")

	return ok && startsNewWord(rest)
}

func inTestFile(pass *analysis.Pass, node ast.Node) bool {
	return strings.HasSuffix(pass.Fset.File(node.Pos()).Name(), "_test.go")
}

func hasTestSignature(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if !ok {
		return false
	}

	sig := obj.Signature()

	return sig.Recv() == nil &&
		sig.TypeParams().Len() == 0 &&
		sig.Results().Len() == 0 &&
		sig.Params().Len() == 1 &&
		isPointerTo(sig.Params().At(0).Type(), "testing", "T")
}

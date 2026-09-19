package semcheck

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// A Match is a node selected by a Matcher.
type Match struct {
	Node ast.Node
	Pos  token.Pos // where findings about Node are reported
}

// A Matcher selects, deterministically, the nodes a rule asks about.
type Matcher struct {
	Name  string
	Types []ast.Node // the node types Match is offered, as in inspector filters
	Match func(pass *analysis.Pass, cur inspector.Cursor) (Match, bool)
}

// matches returns the nodes of the package selected by m, in source order.
func (m *Matcher) matches(pass *analysis.Pass) []Match {
	in := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	var found []Match

	for fileCur := range in.Root().Children() {
		// Includes the main package that "go test" synthesizes.
		if ast.IsGenerated(fileCur.Node().(*ast.File)) {
			continue
		}

		for cur := range fileCur.Preorder(m.Types...) {
			if match, ok := m.Match(pass, cur); ok {
				found = append(found, match)
			}
		}
	}

	return found
}

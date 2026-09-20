package semcheck

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// A match is a node selected by a matcher.
type match struct {
	Node ast.Node
	Pos  token.Pos // where findings about Node are reported

	// Func is the innermost *ast.FuncDecl or *ast.FuncLit that contains Node,
	// or Node itself if it is one. It is nil for nodes at package level.
	Func ast.Node

	// Stmt is the innermost statement that contains Node, or nil.
	Stmt ast.Stmt
}

// A matcher selects, deterministically, the nodes a rule asks about.
type matcher struct {
	Name  string
	Types []ast.Node // the node types Match is offered, as in inspector filters
	Match func(pass *analysis.Pass, cur inspector.Cursor) (match, bool)

	// ForTests marks a matcher whose whole point is test code.
	ForTests bool
}

// matches returns the nodes of the package selected by m, in source order.
func (m *matcher) matches(pass *analysis.Pass) []match {
	in := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	var found []match

	for fileCur := range in.Root().Children() {
		// Includes the main package that "go test" synthesizes.
		if ast.IsGenerated(fileCur.Node().(*ast.File)) {
			continue
		}

		for cur := range fileCur.Preorder(m.Types...) {
			if hit, ok := m.Match(pass, cur); ok {
				hit.Func = enclosingFunc(cur)
				hit.Stmt = enclosingStmt(cur)
				found = append(found, hit)
			}
		}
	}

	return found
}

func enclosingFunc(cur inspector.Cursor) ast.Node {
	for c := range cur.Enclosing((*ast.FuncDecl)(nil), (*ast.FuncLit)(nil)) {
		return c.Node()
	}

	return nil
}

func enclosingStmt(cur inspector.Cursor) ast.Stmt {
	for c := range cur.Enclosing() {
		if stmt, ok := c.Node().(ast.Stmt); ok {
			return stmt
		}
	}

	return nil
}

// must is for matchers built from constants, like regexp.MustCompile.
func must(m *matcher, err error) *matcher {
	if err != nil {
		panic(err)
	}

	return m
}

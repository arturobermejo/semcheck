package semcheck

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// exportedFuncDoc selects the exported functions and methods that have a doc
// comment.
var exportedFuncDoc = &Matcher{
	Name:  "exported-func-doc",
	Types: []ast.Node{(*ast.FuncDecl)(nil)},
	Match: func(_ *analysis.Pass, cur inspector.Cursor) (Match, bool) {
		fn := cur.Node().(*ast.FuncDecl)
		// Without a body there is no behavior to compare the comment with.
		if fn.Body == nil || !isExportedFunc(fn) || !hasDoc(fn) {
			return Match{}, false
		}

		return Match{Node: fn, Pos: fn.Name.Pos()}, true
	},
}

func isExportedFunc(fn *ast.FuncDecl) bool {
	if !fn.Name.IsExported() {
		return false
	}

	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return true
	}

	// A method is part of the API only if its receiver type is.
	recv := receiverTypeName(fn.Recv.List[0].Type)

	return recv != nil && recv.IsExported()
}

// receiverTypeName unwraps a receiver type such as *T, T[K] or *T[K, V] down
// to T. It returns nil for anything else.
func receiverTypeName(expr ast.Expr) *ast.Ident {
	for {
		switch t := expr.(type) {
		case *ast.Ident:
			return t
		case *ast.StarExpr:
			expr = t.X
		case *ast.ParenExpr:
			expr = t.X
		case *ast.IndexExpr:
			expr = t.X
		case *ast.IndexListExpr:
			expr = t.X
		default:
			return nil
		}
	}
}

// hasDoc ignores doc comments made only of directives such as //go:noinline:
// CommentGroup.Text drops them.
func hasDoc(fn *ast.FuncDecl) bool {
	return strings.TrimSpace(fn.Doc.Text()) != ""
}

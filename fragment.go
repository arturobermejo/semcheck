package semcheck

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/printer"

	"golang.org/x/tools/go/analysis"
)

// A Context says how much code around a match the model gets to see.
type Context string

const (
	// ContextStatement is the statement that contains the match.
	ContextStatement Context = "statement"
	// ContextFunction is the whole function that contains the match.
	ContextFunction Context = "function"
)

// maxFragmentBytes is far above anything found in real code (the largest of
// 3,261 fragments measured was under 14 KiB) and keeps a generated monster from
// becoming a question.
const maxFragmentBytes = 16 << 10

// errFragmentTooLarge lets the caller skip a match instead of failing. Cutting
// the text is not an option: the part left out may be the one that matters.
var errFragmentTooLarge = errors.New("fragment too large")

// scope returns the node that stands for m in the given context. When there is
// no function or statement around m, as for a declaration or a call at package
// level, it is the matched node itself.
func (m Match) scope(ctx Context) ast.Node {
	switch {
	case ctx == ContextFunction && m.Func != nil:
		return m.Func
	case ctx == ContextStatement && m.Stmt != nil:
		return m.Stmt
	default:
		return m.Node
	}
}

// fragment renders the code a question about m is asked on. It prints the
// syntax tree instead of copying the file, so that the text does not depend on
// the indentation or the spacing of the original, and on nothing outside the
// node: the same code gives the same fragment, which is what makes it cacheable.
func fragment(pass *analysis.Pass, m Match, ctx Context) (string, error) {
	node := m.scope(ctx)

	// A plain node is printed without the comments inside it: they are not
	// part of the tree, but of the file.
	var comments []*ast.CommentGroup

	for _, f := range pass.Files {
		if f.FileStart <= node.Pos() && node.Pos() < f.FileEnd {
			comments = f.Comments
		}
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, pass.Fset, &printer.CommentedNode{Node: node, Comments: comments}); err != nil {
		return "", fmt.Errorf("semcheck: printing the code at %s: %w", pass.Fset.Position(node.Pos()), err)
	}

	if buf.Len() > maxFragmentBytes {
		return "", fmt.Errorf("semcheck: the code at %s takes %d bytes: %w", pass.Fset.Position(node.Pos()), buf.Len(), errFragmentTooLarge)
	}

	return buf.String(), nil
}

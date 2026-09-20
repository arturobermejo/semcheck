package semcheck

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// nolint knows the lines of a package where //nolint directives silence
// semcheck. Only golangci-lint understands those directives: without this the
// standalone command would ignore them. Looking before asking also keeps
// silenced code from becoming questions.
//
// The rules are the ones golangci-lint was seen to follow. A directive
// silences its own line. If it is in a comment that has its lines to itself,
// it also silences the whole node that starts right below the comment: a
// statement, a block, a function, or the file when it is above the package
// clause.
type nolint map[*token.File][]lineRange

type lineRange struct{ from, to int }

func newNolint(pass *analysis.Pass) nolint {
	silenced := nolint{}

	for _, file := range pass.Files {
		if ranges := nolintRanges(pass.Fset, file); len(ranges) > 0 {
			silenced[pass.Fset.File(file.Pos())] = ranges
		}
	}

	return silenced
}

func (n nolint) covers(fset *token.FileSet, pos token.Pos) bool {
	line := fset.Position(pos).Line

	for _, r := range n[fset.File(pos)] {
		if r.from <= line && line <= r.to {
			return true
		}
	}

	return false
}

func nolintRanges(fset *token.FileSet, file *ast.File) []lineRange {
	line := func(pos token.Pos) int { return fset.Position(pos).Line }

	var (
		ranges []lineRange
		groups []*ast.CommentGroup // the ones with a directive
	)

	for _, group := range file.Comments {
		found := false

		for _, c := range group.List {
			if silencesSemcheck(c.Text) {
				ranges = append(ranges, lineRange{line(c.Pos()), line(c.Pos())})
				found = true
			}
		}

		if found {
			groups = append(groups, group)
		}
	}

	if len(groups) == 0 {
		return nil // nearly every file: spare it the two walks below
	}

	// A comment has its lines to itself if no code starts or ends on the first.
	code := map[int]bool{}

	walk(file, func(n ast.Node) {
		code[line(n.Pos())] = true
		code[line(n.End())] = true
	})

	below := map[int]bool{} // lines where a silenced node may start

	for _, group := range groups {
		if !code[line(group.Pos())] {
			below[line(group.End())+1] = true
		}
	}

	walk(file, func(n ast.Node) {
		if from := line(n.Pos()); below[from] {
			ranges = append(ranges, lineRange{from, line(n.End())})
		}
	})

	return ranges
}

// walk visits the nodes of a file that are code, not comments.
func walk(file *ast.File, visit func(ast.Node)) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch n.(type) {
		case nil:
			return false
		case *ast.Comment, *ast.CommentGroup:
			return false
		}

		visit(n)

		return true
	})
}

// silencesSemcheck reports whether the text of a comment is a directive such as
//
//	//nolint
//	//nolint:semcheck // with a reason
//	// nolint:errcheck, SEMCHECK
func silencesSemcheck(comment string) bool {
	text, ok := strings.CutPrefix(comment, "//")
	if !ok {
		return false // a /* block */ comment
	}

	text, ok = strings.CutPrefix(strings.TrimSpace(text), "nolint")
	if !ok {
		return false
	}

	// What follows a second // is the reason.
	text, _, _ = strings.Cut(text, "//")
	text = strings.TrimSpace(text)

	if text == "" {
		return true // every linter
	}

	list, ok := strings.CutPrefix(text, ":")
	if !ok {
		return false // "nolinter", "nolint semcheck"
	}

	for _, name := range strings.Split(list, ",") {
		if name = strings.ToLower(strings.TrimSpace(name)); name == analyzerName || name == "all" {
			return true
		}
	}

	return false
}

package semcheck

import (
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"path"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/types/typeutil"
)

// callTo selects the calls to functions and methods that match one of the
// patterns, such as "log.*", "slog.Info" or "go.uber.org/zap.*".
//
// Of several matching calls nested in one another, as in a fluent chain
// log.Info().Str("k", v).Msg("m") or in slog.Info("m", slog.String("k", v)),
// only the outermost is selected: it contains the others.
func callTo(patterns ...string) (*Matcher, error) {
	if len(patterns) == 0 {
		return nil, errors.New("call needs at least one pattern")
	}

	parsed := make([]callPattern, len(patterns))

	for i, p := range patterns {
		var err error
		if parsed[i], err = parseCallPattern(p); err != nil {
			return nil, err
		}
	}

	matches := func(pass *analysis.Pass, call *ast.CallExpr) bool {
		fn, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)

		return ok && slices.ContainsFunc(parsed, func(p callPattern) bool { return p.matches(fn) })
	}

	return &Matcher{
		Name:  "call",
		Types: []ast.Node{(*ast.CallExpr)(nil)},
		Match: func(pass *analysis.Pass, cur inspector.Cursor) (Match, bool) {
			call := cur.Node().(*ast.CallExpr)
			if !matches(pass, call) {
				return Match{}, false
			}

			for c := range cur.Parent().Enclosing((*ast.CallExpr)(nil), (*ast.FuncLit)(nil)) {
				outer, ok := c.Node().(*ast.CallExpr)
				if !ok {
					break // a function literal is a context of its own
				}

				if matches(pass, outer) {
					return Match{}, false
				}
			}

			return Match{Node: call, Pos: call.Pos()}, true
		},
	}, nil
}

// A callPattern is "pkg.Func": pkg is the path or the name of the package that
// declares the function or method, Func a glob for its name.
type callPattern struct {
	pkg, fn string
}

func parseCallPattern(s string) (callPattern, error) {
	i := strings.LastIndex(s, ".")
	if i <= 0 || i == len(s)-1 {
		return callPattern{}, fmt.Errorf("call: %q must look like pkg.Func or pkg.*", s)
	}

	p := callPattern{pkg: s[:i], fn: s[i+1:]}

	// A slash means the dot was inside the import path: "go.uber.org/zap".
	if _, err := path.Match(p.fn, ""); err != nil || strings.Contains(p.fn, "/") {
		return callPattern{}, fmt.Errorf("call: %q must look like pkg.Func or pkg.*", s)
	}

	return p, nil
}

func (p callPattern) matches(fn *types.Func) bool {
	pkg := fn.Pkg()
	if pkg == nil || (p.pkg != pkg.Path() && p.pkg != pkg.Name()) {
		return false
	}

	ok, _ := path.Match(p.fn, fn.Name())

	return ok
}

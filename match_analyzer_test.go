package semcheck

import (
	"cmp"
	"go/token"
	"slices"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

// newMatchAnalyzer returns an analysis that reports every node the matchers
// select, to test them without rules or a judge.
func newMatchAnalyzer(matchers ...*matcher) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "semcheck",
		Doc:      "reports the nodes selected by the semcheck matchers",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			type finding struct {
				pos     token.Pos
				matcher string
			}

			var findings []finding

			for _, m := range matchers {
				for _, hit := range m.matches(pass) {
					findings = append(findings, finding{hit.Pos, m.Name})
				}
			}

			// Drivers print diagnostics in the order they are reported.
			slices.SortFunc(findings, func(a, b finding) int {
				return cmp.Or(cmp.Compare(a.pos, b.pos), cmp.Compare(a.matcher, b.matcher))
			})

			for _, f := range findings {
				pass.Reportf(f.pos, "%s: matched", f.matcher)
			}

			return nil, nil
		},
	}
}

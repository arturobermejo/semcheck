package semcheck

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

// Analyzer is the semcheck analysis. There are no rules yet: it reports every
// node selected by the matchers.
var Analyzer = newMatchAnalyzer(exportedFuncDoc)

func newMatchAnalyzer(matchers ...*Matcher) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "semcheck",
		Doc:      "reports the nodes selected by the semcheck matchers",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			for _, m := range matchers {
				for _, match := range m.matches(pass) {
					pass.Reportf(match.Pos, "%s: matched", m.Name)
				}
			}

			return nil, nil
		},
	}
}

package semcheck

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzerValid(t *testing.T) {
	if err := analysis.Validate([]*analysis.Analyzer{Analyzer}); err != nil {
		t.Fatal(err)
	}
}

// TestAnalyzer runs the analyzer on the packages under testdata/src and
// compares its diagnostics with the "// want" comments in those files.
func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "funcs", "nofuncs", "generated")
}

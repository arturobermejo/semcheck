package semcheck

import (
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestAnalyzerValid(t *testing.T) {
	if err := analysis.Validate([]*analysis.Analyzer{Analyzer}); err != nil {
		t.Fatal(err)
	}
}

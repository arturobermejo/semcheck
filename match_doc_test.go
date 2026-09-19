package semcheck

import (
	"go/parser"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestExportedFuncDoc(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), newMatchAnalyzer(exportedFuncDoc), "exporteddoc")
}

func TestReceiverTypeName(t *testing.T) {
	tests := []struct {
		expr string
		want string // "" when there is no type name
	}{
		{"T", "T"},
		{"*T", "T"},
		{"(T)", "T"},
		{"(*T)", "T"},
		{"T[K]", "T"},
		{"*T[K]", "T"},
		{"*T[K, V]", "T"},
		{"pkg.T", ""},
		{"[]T", ""},
		{"func()", ""},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			expr, err := parser.ParseExpr(tt.expr)
			if err != nil {
				t.Fatal(err)
			}

			got := ""
			if name := receiverTypeName(expr); name != nil {
				got = name.Name
			}

			if got != tt.want {
				t.Errorf("receiverTypeName(%s) = %q, want %q", tt.expr, got, tt.want)
			}
		})
	}
}

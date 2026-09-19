package semcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ast/inspector"
)

func TestFuncPrefix(t *testing.T) {
	m, err := funcPrefix("Get", "Is", "Has")
	if err != nil {
		t.Fatal(err)
	}

	analysistest.Run(t, analysistest.TestData(), newMatchAnalyzer(m), "funcprefix")
}

func TestFuncPrefixInvalid(t *testing.T) {
	tests := []struct {
		name     string
		prefixes []string
	}{
		{"no prefixes", nil},
		{"empty prefix", []string{"Get", ""}},
		{"not an identifier", []string{"get-user"}},
		{"starts with a digit", []string{"2Get"}},
		{"contains a space", []string{"Get User"}},
		{"keyword", []string{"func"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := funcPrefix(tt.prefixes...)
			if err == nil {
				t.Fatalf("funcPrefix(%q) succeeded, want an error", tt.prefixes)
			}

			if m != nil {
				t.Errorf("funcPrefix(%q) returned a matcher along with the error", tt.prefixes)
			}
		})
	}
}

func TestFuncPrefixCopiesItsArguments(t *testing.T) {
	prefixes := []string{"Get"}

	m, err := funcPrefix(prefixes...)
	if err != nil {
		t.Fatal(err)
	}

	prefixes[0] = "Set" // the caller reuses its slice

	file, err := parser.ParseFile(token.NewFileSet(), "p.go", "package p\n\nfunc GetUser() {}\n", parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}

	in := inspector.New([]*ast.File{file})

	for cur := range in.Root().Preorder(m.Types...) {
		if _, ok := m.Match(nil, cur); !ok {
			t.Error("GetUser is no longer selected: the matcher shares the caller's slice")
		}
	}
}

func TestHasWordPrefix(t *testing.T) {
	tests := []struct {
		name, prefix string
		want         bool
	}{
		{"GetUser", "Get", true},
		{"Get", "Get", true},
		{"getUser", "Get", true},
		{"GetUser", "get", true},
		{"Get2FA", "Get", true},
		{"Get_user", "Get", true},
		{"IsNaN", "Is", true},
		{"ñandúCorre", "Ñandú", true},

		{"Getter", "Get", false},
		{"Issue", "Is", false},
		{"Hash", "Has", false},
		{"Listen", "List", false},
		{"ForgetUser", "Get", false},
		{"GETUser", "Get", false},
		{"Ge", "Get", false},
		{"", "Get", false},
		{"Ñandúes", "Ñandú", false},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.prefix, func(t *testing.T) {
			if got := hasWordPrefix(tt.name, tt.prefix); got != tt.want {
				t.Errorf("hasWordPrefix(%q, %q) = %v, want %v", tt.name, tt.prefix, got, tt.want)
			}
		})
	}
}

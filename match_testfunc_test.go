package semcheck

import (
	"go/ast"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestTestFunc(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), newMatchAnalyzer(testFunc), "testfunc")
}

func TestIsTestName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Test", true},
		{"TestLogin", true},
		{"Test_login", true},
		{"Test2FA", true},
		{"TestÑandú", true},

		{"Testify", false},
		{"Testñandú", false},
		{"testLogin", false},
		{"LoginTest", false},
		{"Tes", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTestName(tt.name); got != tt.want {
				t.Errorf("isTestName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// "go test" rejects a TestXxx with the wrong signature, so these cases cannot
// live in testdata. Other drivers still analyze such code, e.g. while it is
// being written.
func TestHasTestSignature(t *testing.T) {
	imp := fakeImporter{
		"testing":     fakePackage("testing", "testing", "T", "B", "M"),
		"faketesting": fakePackage("faketesting", "testing", "T"),
	}

	tests := []struct {
		name    string
		imports string
		decl    string
		want    bool
	}{
		{"test", `"testing"`, "func TestX(t *testing.T) {}", true},
		{"renamed import", `tt "testing"`, "func TestX(t *tt.T) {}", true},
		{"unnamed parameter", `"testing"`, "func TestX(*testing.T) {}", true},
		{"dot import", `. "testing"`, "func TestX(t *T) {}", true},
		{"type alias", `"testing"`, "type myT = testing.T; func TestX(t *myT) {}", true},
		{"alias of the pointer", `"testing"`, "type ptr = *testing.T; func TestX(t ptr) {}", true},

		{"impostor package", `"faketesting"`, "func TestX(t *testing.T) {}", false},
		{"benchmark type", `"testing"`, "func TestX(b *testing.B) {}", false},
		{"TestMain", `"testing"`, "func TestMain(m *testing.M) {}", false},
		{"defined type, not an alias", `"testing"`, "type myT testing.T; func TestX(t *myT) {}", false},
		{"not a pointer", `"testing"`, "func TestX(t testing.T) {}", false},
		{"pointer to pointer", `"testing"`, "func TestX(t **testing.T) {}", false},
		{"no parameters", `"testing"`, "var _ testing.T; func TestX() {}", false},
		{"extra parameter", `"testing"`, "func TestX(t *testing.T, name string) {}", false},
		{"two in one field", `"testing"`, "func TestX(a, b *testing.T) {}", false},
		{"result", `"testing"`, "func TestX(t *testing.T) error { return nil }", false},
		{"type parameter", `"testing"`, "func TestX[V any](t *testing.T) {}", false},
		{"method", `"testing"`, "type s struct{}; func (s) TestX(t *testing.T) {}", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, pass := typeCheck(t, "package p\n\nimport "+tt.imports+"\n\n"+tt.decl+"\n", imp)

			// The function under test is the last declaration.
			fn := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)

			if got := hasTestSignature(pass, fn); got != tt.want {
				t.Errorf("hasTestSignature(%s) = %v, want %v", tt.decl, got, tt.want)
			}
		})
	}
}

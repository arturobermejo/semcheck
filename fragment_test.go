package semcheck

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// matchAt parses src and builds the Match for its first node accepted by pick,
// the same way Matcher.matches does.
func matchAt(t *testing.T, src string, pick func(ast.Node) bool, otherFiles ...string) (*analysis.Pass, Match) {
	t.Helper()

	pass := &analysis.Pass{Fset: token.NewFileSet()}

	for i, src := range append([]string{src}, otherFiles...) {
		file, err := parser.ParseFile(pass.Fset, fmt.Sprintf("p%d.go", i), src, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}

		pass.Files = append(pass.Files, file)
	}

	for cur := range inspector.New(pass.Files).Root().Preorder() {
		if pick(cur.Node()) {
			return pass, Match{
				Node: cur.Node(),
				Pos:  cur.Node().Pos(),
				Func: enclosingFunc(cur),
				Stmt: enclosingStmt(cur),
			}
		}
	}

	t.Fatal("no node was picked")

	return nil, Match{}
}

// isCall picks the call whose function is written as name, e.g. "log.Printf".
func isCall(name string) func(ast.Node) bool {
	return func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return false
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}

		pkg, ok := sel.X.(*ast.Ident)

		return ok && pkg.Name+"."+sel.Sel.Name == name
	}
}

func isFunc(name string) func(ast.Node) bool {
	return func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)

		return ok && fn.Name.Name == name
	}
}

const fragmentSource = `package p

// Save stores the user.
//
//go:noinline
func Save(u User) error {
			// validate first
			if u.ID==0 {   return errEmpty }

			log.Printf( "saving %v",
				u )   // after the call

			go func() {
				defer audit.Record("saved", u.Email)
			}()

			return   nil
}

var _ = log.Prefix()

// after Save
`

func TestFragment(t *testing.T) {
	tests := []struct {
		name string
		pick func(ast.Node) bool
		ctx  Context
		want string
	}{
		{
			name: "a statement is printed from column zero, reformatted",
			pick: isCall("log.Printf"),
			ctx:  ContextStatement,
			want: `log.Printf("saving %v",
	u)`,
		},
		{
			name: "the statement around a call, not the call alone",
			pick: isCall("audit.Record"),
			ctx:  ContextStatement,
			want: `defer audit.Record("saved", u.Email)`,
		},
		{
			name: "a function comes with its doc and the comments inside it",
			pick: isCall("log.Printf"),
			ctx:  ContextFunction,
			want: `// Save stores the user.
//
//go:noinline
func Save(u User) error {
	// validate first
	if u.ID == 0 {
		return errEmpty
	}

	log.Printf("saving %v",
		u) // after the call

	go func() {
		defer audit.Record("saved", u.Email)
	}()

	return nil
}`,
		},
		{
			name: "the function is the innermost one",
			pick: isCall("audit.Record"),
			ctx:  ContextFunction,
			want: `func() {
	defer audit.Record("saved", u.Email)
}`,
		},
		{
			name: "a function matched as a whole is its own statement",
			pick: isFunc("Save"),
			ctx:  ContextStatement,
			want: "", // same as the function context: checked below
		},
		{
			name: "at package level there is only the node",
			pick: isCall("log.Prefix"),
			ctx:  ContextFunction,
			want: `log.Prefix()`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pass, m := matchAt(t, fragmentSource, tt.pick)

			got, err := fragment(pass, m, tt.ctx)
			if err != nil {
				t.Fatal(err)
			}

			want := tt.want
			if want == "" {
				if want, err = fragment(pass, m, ContextFunction); err != nil {
					t.Fatal(err)
				}
			}

			if got != want {
				t.Errorf("fragment:\n%s\n\nwant:\n%s", got, want)
			}
		})
	}
}

// The text must depend on the code alone: not on where it is in the file, how
// deep it is nested or how it was spaced.
func TestFragmentIgnoresLayout(t *testing.T) {
	const tidy = `package p

func f() {
	log.Printf("saving %v", u)
}
`

	const messy = `package p

import "log"

type unrelated struct{}

func g() {
	if true {
		for {
				log.Printf(  "saving %v",u  )
		}
	}
}
`

	var got []string

	for _, src := range []string{tidy, messy} {
		pass, m := matchAt(t, src, isCall("log.Printf"))

		text, err := fragment(pass, m, ContextStatement)
		if err != nil {
			t.Fatal(err)
		}

		got = append(got, text)
	}

	if got[0] != got[1] {
		t.Errorf("fragments differ:\n%s\n\n%s", got[0], got[1])
	}
}

func TestFragmentKeepsWhatChangesTheMeaning(t *testing.T) {
	variants := []string{
		`log.Printf("saving %v", u)`,
		`log.Printf("saving %v", u.Email)`,
		`log.Printf("saving %+v", u)`,
		"log.Printf(\"saving %v\", u) // u has no personal data",
	}

	seen := map[string]string{}

	for _, stmt := range variants {
		pass, m := matchAt(t, "package p\n\nfunc f() {\n\t"+stmt+"\n\t_ = 0\n}\n", isCall("log.Printf"))

		text, err := fragment(pass, m, ContextFunction)
		if err != nil {
			t.Fatal(err)
		}

		if other, ok := seen[text]; ok {
			t.Errorf("%q and %q give the same fragment", stmt, other)
		}

		seen[text] = stmt

		if !strings.Contains(text, "log.Printf") {
			t.Errorf("fragment of %q lost the call:\n%s", stmt, text)
		}
	}
}

func TestFragmentTooLarge(t *testing.T) {
	// Each statement takes 13 bytes once printed: a tab, 11 characters, a newline.
	body := func(statements int) string {
		return "package p\n\nfunc big() {\n" + strings.Repeat("\tx = x + 100\n", statements) + "}\n"
	}

	const overhead = len("func big() {\n}")

	fits := (maxFragmentBytes - overhead) / 13

	pass, m := matchAt(t, body(fits), isFunc("big"))
	if text, err := fragment(pass, m, ContextFunction); err != nil {
		t.Fatalf("%d bytes: %v", len(text), err)
	}

	pass, m = matchAt(t, body(fits+1), isFunc("big"))

	text, err := fragment(pass, m, ContextFunction)
	if !errors.Is(err, errFragmentTooLarge) {
		t.Fatalf("got %d bytes and error %v, want errFragmentTooLarge", len(text), err)
	}

	if text != "" {
		t.Error("a fragment came along with the error")
	}

	// The statement is still small: the limit applies to what is printed.
	pass, m = matchAt(t, body(fits+1), func(n ast.Node) bool { _, ok := n.(*ast.AssignStmt); return ok })
	if _, err := fragment(pass, m, ContextStatement); err != nil {
		t.Error(err)
	}
}

// Comments live in their file, not in the tree: a package has several files,
// and the ones of the right file must be found.
func TestFragmentCommentsInEveryFile(t *testing.T) {
	const (
		first  = "package p\n\nfunc first() {\n\t// in the first file\n\tlog.Println(1)\n}\n"
		second = "package p\n\nfunc second() {\n\t// in the second file\n\tlog.Println(2)\n}\n"
	)

	for _, name := range []string{"first", "second"} {
		pass, m := matchAt(t, first, isFunc(name), second)

		text, err := fragment(pass, m, ContextFunction)
		if err != nil {
			t.Fatal(err)
		}

		if want := "// in the " + name + " file"; !strings.Contains(text, want) {
			t.Errorf("the fragment of %s lost its comment %q:\n%s", name, want, text)
		}
	}
}

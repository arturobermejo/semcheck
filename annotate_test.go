package semcheck

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// storePackage is a double of a database package: a DB whose fields are all
// unexported, as in database/sql, and a Row with one field of each kind.
func storePackage() *types.Package {
	pkg := types.NewPackage("example.com/store", "store")

	declare := func(name string, fields ...*types.Var) {
		obj := types.NewTypeName(token.NoPos, pkg, name, nil)
		types.NewNamed(obj, types.NewStruct(fields, nil), nil)
		pkg.Scope().Insert(obj)
	}

	field := func(name string, typ types.Type) *types.Var {
		return types.NewField(token.NoPos, pkg, name, typ, false)
	}

	declare("DB", field("conn", types.Typ[types.Int]), field("mu", types.Typ[types.Bool]))
	declare("Row", field("Email", types.Typ[types.String]), field("cached", types.Typ[types.Bool]))
	pkg.MarkComplete()

	return pkg
}

const notesPreamble = `package p

import "example.com/store"

type Contact struct {
	Email string
	Phone string
}

type User struct {
	ID      int
	Email   string
	Contact Contact
	secret  string
}

type Node struct {
	Value int
	Next  *Node
}

type Level string

type Person = User

type ID = string

type Page[T any] struct {
	Items []T
}

type Embeds struct {
	Contact
	Name string
}

// Wide has more fields than a nested struct may show.
type Wide struct {
	A, B, C, D, E, F, G, H, I int
}

type Holder struct {
	Wide    Wide
	Contact Contact
}

type Logger struct {
	prefix string
}

func (l *Logger) With(args ...any) *Logger { return l }

func (l *Logger) Info(args ...any) {}

var global *store.DB

func log(args ...any) {}

`

// notesOf type-checks a function body and returns the notes for the first
// statement of it that is a call, which stands for the matched node.
func notesOf(t *testing.T, params, body string) []string {
	t.Helper()

	src := notesPreamble + "func f(" + params + ") {\n" + body + "\n}\n"
	file, pass := typeCheck(t, src, fakeImporter{"example.com/store": storePackage()})

	var call ast.Node

	ast.Inspect(file, func(n ast.Node) bool {
		if stmt, ok := n.(*ast.ExprStmt); ok && call == nil {
			call, _ = stmt.X.(*ast.CallExpr)
		}

		return call == nil
	})

	if call == nil {
		t.Fatal("no statement is a call")
	}

	return typeNotes(pass, match{Node: call})
}

func TestTypeNotes(t *testing.T) {
	tests := []struct {
		name   string
		params string
		body   string
		want   []string
	}{
		{
			name:   "a struct is spelled out two levels deep",
			params: "u User",
			body:   `log("created %+v", u)`,
			want:   []string{"u: User{ID int; Email string; Contact Contact{Email string; Phone string}; secret string}"},
		},
		{
			name:   "a selector is described through its variable",
			params: "u *User",
			body:   `log("contact", u.Contact)`,
			want:   []string{"u: *User{ID int; Email string; Contact Contact{Email string; Phone string}; secret string}"},
		},
		{
			name:   "what the code already says is left out",
			params: "fullName string, n int, err error, ok bool, raw []byte",
			body:   `log(fullName, n, err, ok, raw)`,
			want:   nil,
		},
		{
			name:   "a named type without fields keeps its name",
			params: "lvl Level",
			body:   `log(lvl)`,
			want:   []string{"lvl: Level"},
		},
		{
			name:   "an alias is described as what it stands for",
			params: "p Person, ps []*Person, id ID",
			body:   `log(p, ps, id)`,
			want: []string{
				"p: User{ID int; Email string; Contact Contact{Email string; Phone string}; secret string}",
				"ps: []*User{ID int; Email string; Contact Contact{Email string; Phone string}; secret string}",
			},
		},
		{
			name:   "other packages are qualified and show what is exported",
			params: "db *store.DB, row store.Row",
			body:   `log(db, row)`,
			want:   []string{"db: *store.DB", "row: store.Row{Email string}"},
		},
		{
			name:   "package level variables count",
			params: "",
			body:   `log(global)`,
			want:   []string{"global: *store.DB"},
		},
		{
			name:   "containers are described by their elements",
			params: "users []User, byID map[int]*Contact, pair [2]Contact",
			body:   `log(users, byID, pair)`,
			want: []string{
				"users: []User{ID int; Email string; Contact Contact{Email string; Phone string}; secret string}",
				"byID: map[int]*Contact{Email string; Phone string}",
				"pair: [2]Contact{Email string; Phone string}",
			},
		},
		{
			name:   "a recursive type stops at the depth limit",
			params: "n *Node",
			body:   `log(n)`,
			want:   []string{"n: *Node{Value int; Next *Node{Value int; Next *Node}}"},
		},
		{
			name:   "a generic type is described with its arguments",
			params: "p Page[Contact]",
			body:   `log(p)`,
			want:   []string{"p: Page[Contact]{Items []Contact{Email string; Phone string}}"},
		},
		{
			name:   "an embedded field goes by the name of its type",
			params: "e Embeds",
			body:   `log(e)`,
			want:   []string{"e: Embeds{Contact Contact{Email string; Phone string}; Name string}"},
		},
		{
			name:   "an anonymous struct is a type too",
			params: "",
			body:   "anon := struct{ Email string }{}\nlog(anon)",
			want:   []string{"anon: struct{Email string}"},
		},
		{
			name:   "each variable once, in order of appearance",
			params: "b Contact, a Contact",
			body:   `log(b, a, b.Email, a.Phone)`,
			want:   []string{"b: Contact{Email string; Phone string}", "a: Contact{Email string; Phone string}"},
		},
		{
			name:   "variables of a closure inside the node count",
			params: "u User",
			body:   `log(func() any { return u.Email }())`,
			want:   []string{"u: User{ID int; Email string; Contact Contact{Email string; Phone string}; secret string}"},
		},
		{
			name:   "the receiver of the matched call says nothing about what is logged",
			params: "logger *Logger, u User, c Contact",
			body:   `logger.With("user", u).Info("saved", c)`,
			want: []string{
				"u: User{ID int; Email string; Contact Contact{Email string; Phone string}; secret string}",
				"c: Contact{Email string; Phone string}",
			},
		},
		{
			name:   "a receiver inside an argument is a value like any other",
			params: "logger *Logger, c Contact",
			body:   `log(logger, c)`,
			want:   []string{"logger: *Logger{prefix string}", "c: Contact{Email string; Phone string}"},
		},
		{
			name:   "a wide struct is spelled out on its own but not inside another",
			params: "w Wide, h Holder",
			body:   `log(w, h)`,
			want: []string{
				"w: Wide{A int; B int; C int; D int; E int; F int; G int; H int; I int}",
				"h: Holder{Wide Wide; Contact Contact{Email string; Phone string}}",
			},
		},
		{
			name:   "only the matched node is looked at",
			params: "u User, c Contact",
			body:   "_ = u\nlog(c)",
			want:   []string{"c: Contact{Email string; Phone string}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notesOf(t, tt.params, tt.body); !slices.Equal(got, tt.want) {
				t.Errorf("notes:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(tt.want, "\n  "))
			}
		})
	}
}

func TestTypeNotesAreCapped(t *testing.T) {
	var params, args []string

	for i := range maxTypeNotes + 5 {
		name := "c" + strconv.Itoa(i)
		params = append(params, name+" Contact")
		args = append(args, name)
	}

	got := notesOf(t, strings.Join(params, ", "), "log("+strings.Join(args, ", ")+")")

	if len(got) != maxTypeNotes {
		t.Fatalf("got %d notes, want %d", len(got), maxTypeNotes)
	}

	if first, last := got[0], got[len(got)-1]; !strings.HasPrefix(first, "c0:") || !strings.HasPrefix(last, "c"+strconv.Itoa(maxTypeNotes-1)+":") {
		t.Errorf("the notes kept are %q … %q, want the first %d variables", first, last, maxTypeNotes)
	}
}

func TestDescribeFieldsAreCapped(t *testing.T) {
	pkg := types.NewPackage("p", "p")

	var fields []*types.Var
	for i := range maxNoteFields + 3 {
		fields = append(fields, types.NewField(token.NoPos, pkg, "F"+strconv.Itoa(i), types.Typ[types.Int], false))
	}

	wide := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Wide", nil), types.NewStruct(fields, nil), nil)

	got := describeType(pkg, wide, noteDepth)

	if n := strings.Count(got, ";") + 1; n != maxNoteFields+1 || !strings.HasSuffix(got, "; …}") {
		t.Errorf("describeType = %s\nwant %d fields and a final …", got, maxNoteFields)
	}
}

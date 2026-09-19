package semcheck

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const (
	maxTypeNotes  = 16
	maxNoteFields = 16

	// A struct inside another one is spelled out only if it is small. Domain
	// types such as Contact{Email; Phone} are; *http.Transport, with forty
	// fields, made a single note thirty times longer than its fragment.
	maxNestedFields = 8

	// noteDepth is how many levels of struct fields are spelled out: enough for
	// u and u.Contact in log.Printf("%+v", u), and a bound for recursive types.
	noteDepth = 2
)

// typeNotes lists, as "name: type" lines in order of appearance, the variables
// used in the matched node whose type says something the code does not: that u
// is a User with an Email field, or db a *sql.DB. The model sees text only.
func typeNotes(pass *analysis.Pass, m Match) []string {
	var (
		notes []string
		seen  = map[*types.Var]bool{}
	)

	visit := func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}

		// Fields are described along with the struct they belong to.
		v, ok := pass.TypesInfo.ObjectOf(id).(*types.Var)
		if !ok || v.IsField() || seen[v] || !informative(v.Type()) {
			return true
		}

		seen[v] = true

		if len(notes) < maxTypeNotes {
			notes = append(notes, id.Name+": "+describeType(pass.Pkg, v.Type(), noteDepth))
		}

		return true
	}

	for _, root := range noteRoots(m.Node) {
		ast.Inspect(root, visit)
	}

	return notes
}

// noteRoots returns the parts of node worth describing. For a call they are
// its arguments and those of the calls it is chained to, as in
// logger.With("user", u).Info("saved"), but not the receiver: what logger is
// says nothing about what gets logged.
func noteRoots(node ast.Node) []ast.Node {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return []ast.Node{node}
	}

	var roots []ast.Node

	for call != nil {
		// Inner calls come first in the source: keep the order of appearance.
		args := make([]ast.Node, len(call.Args))
		for i, arg := range call.Args {
			args[i] = arg
		}

		roots = append(args, roots...)

		sel, _ := call.Fun.(*ast.SelectorExpr)
		if sel == nil {
			break
		}

		call, _ = ast.Unparen(sel.X).(*ast.CallExpr)
	}

	return roots
}

// informative reports whether t involves a type declared in some package.
// Basic types, error and the like add nothing to what the code already says.
func informative(t types.Type) bool {
	switch t := types.Unalias(t).(type) {
	case *types.Named:
		return t.Obj().Pkg() != nil
	case *types.Struct:
		return true
	case *types.Pointer:
		return informative(t.Elem())
	case *types.Slice:
		return informative(t.Elem())
	case *types.Array:
		return informative(t.Elem())
	case *types.Chan:
		return informative(t.Elem())
	case *types.Map:
		return informative(t.Key()) || informative(t.Elem())
	default:
		return false
	}
}

// describeType writes t as the package from would, with the fields of its
// struct types spelled out down to depth levels.
func describeType(from *types.Package, t types.Type, depth int) string {
	qualifier := func(pkg *types.Package) string {
		if pkg == from {
			return ""
		}

		return pkg.Name()
	}

	switch t := types.Unalias(t).(type) {
	case *types.Pointer:
		return "*" + describeType(from, t.Elem(), depth)
	case *types.Slice:
		return "[]" + describeType(from, t.Elem(), depth)
	case *types.Array:
		return fmt.Sprintf("[%d]%s", t.Len(), describeType(from, t.Elem(), depth))
	case *types.Map:
		return "map[" + describeType(from, t.Key(), depth) + "]" + describeType(from, t.Elem(), depth)
	case *types.Named:
		name := types.TypeString(t, qualifier)

		st, ok := t.Underlying().(*types.Struct)
		if !ok || depth == 0 {
			return name
		}

		if nested := depth < noteDepth; nested && visibleFields(from, st) > maxNestedFields {
			return name
		}

		if fields := describeFields(from, st, depth-1); fields != "" {
			return name + "{" + fields + "}"
		}

		return name
	default:
		return types.TypeString(t, qualifier)
	}
}

func visibleFields(from *types.Package, st *types.Struct) int {
	n := 0

	for f := range st.Fields() {
		if f.Exported() || f.Pkg() == from {
			n++
		}
	}

	return n
}

// describeFields leaves out the fields that from cannot see: the unexported
// ones of other packages, such as the insides of time.Time or sql.DB.
func describeFields(from *types.Package, st *types.Struct, depth int) string {
	var fields []string

	for f := range st.Fields() {
		if !f.Exported() && f.Pkg() != from {
			continue
		}

		if len(fields) == maxNoteFields {
			fields = append(fields, "…")

			break
		}

		fields = append(fields, f.Name()+" "+describeType(from, f.Type(), depth))
	}

	return strings.Join(fields, "; ")
}

package semcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// fakeImporter resolves imports to packages built by hand, so that a test can
// type-check a snippet without loading the standard library.
type fakeImporter map[string]*types.Package

func (imp fakeImporter) Import(path string) (*types.Package, error) {
	if pkg, ok := imp[path]; ok {
		return pkg, nil
	}

	return nil, fmt.Errorf("package %q not found", path)
}

// fakePackage builds a package that declares empty struct types.
func fakePackage(path, name string, typeNames ...string) *types.Package {
	pkg := types.NewPackage(path, name)

	for _, n := range typeNames {
		obj := types.NewTypeName(token.NoPos, pkg, n, nil)
		types.NewNamed(obj, types.NewStruct(nil, nil), nil)
		pkg.Scope().Insert(obj)
	}

	pkg.MarkComplete()

	return pkg
}

// typeCheck parses and type-checks src, a whole file of a package named p, and
// returns the parts of an analysis.Pass that come from it.
func typeCheck(t *testing.T, src string, imp types.Importer) (*ast.File, *analysis.Pass) {
	t.Helper()

	pass := typeCheckFiles(t, imp, [2]string{"p.go", src})

	return pass.Files[0], pass
}

// typeCheckFiles builds a package out of {name, source} pairs, which get their
// positions in the order given.
func typeCheckFiles(t *testing.T, imp types.Importer, sources ...[2]string) *analysis.Pass {
	t.Helper()

	fset := token.NewFileSet()

	var files []*ast.File

	for _, source := range sources {
		file, err := parser.ParseFile(fset, source[0], source[1], parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}

		files = append(files, file)
	}

	info := &types.Info{
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
		Types: map[ast.Expr]types.TypeAndValue{},
	}

	conf := types.Config{Importer: imp}

	pkg, err := conf.Check("p", fset, files, info)
	if err != nil {
		t.Fatal(err)
	}

	return &analysis.Pass{Fset: fset, Files: files, Pkg: pkg, TypesInfo: info}
}

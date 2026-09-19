package semcheck

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// funcPrefix selects the functions and methods whose name starts with one of
// the given words, such as Get, Is or Has.
func funcPrefix(prefixes ...string) (*Matcher, error) {
	if len(prefixes) == 0 {
		return nil, errors.New("semcheck: func-prefix needs at least one prefix")
	}

	for _, p := range prefixes {
		if !token.IsIdentifier(p) {
			return nil, fmt.Errorf("semcheck: func-prefix: %q is not a valid Go identifier", p)
		}
	}

	prefixes = slices.Clone(prefixes)

	return &Matcher{
		Name:  "func-prefix",
		Types: []ast.Node{(*ast.FuncDecl)(nil)},
		Match: func(_ *analysis.Pass, cur inspector.Cursor) (Match, bool) {
			fn := cur.Node().(*ast.FuncDecl)

			// Without a body there is no behavior to compare the name with.
			if fn.Body == nil {
				return Match{}, false
			}

			if !slices.ContainsFunc(prefixes, func(p string) bool { return hasWordPrefix(fn.Name.Name, p) }) {
				return Match{}, false
			}

			return Match{Node: fn, Pos: fn.Name.Pos()}, true
		},
	}, nil
}

// hasWordPrefix reports whether name starts with the word prefix: IsValid and
// isValid start with Is, Issue does not. The case of the first letter is
// ignored so that unexported names match too.
func hasWordPrefix(name, prefix string) bool {
	first, size := utf8.DecodeRuneInString(name)
	pfirst, psize := utf8.DecodeRuneInString(prefix)

	if unicode.ToLower(first) != unicode.ToLower(pfirst) {
		return false
	}

	rest, ok := strings.CutPrefix(name[size:], prefix[psize:])
	if !ok {
		return false
	}

	return startsNewWord(rest)
}

// startsNewWord reports whether rest, what follows a prefix in a name, is empty
// or begins another word: anything but a lowercase letter.
func startsNewWord(rest string) bool {
	next, _ := utf8.DecodeRuneInString(rest)

	return rest == "" || !unicode.IsLower(next)
}

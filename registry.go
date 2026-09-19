package semcheck

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// matcherRegistry maps the names users write in a "match" field to the
// functions that build the matchers.
var matcherRegistry = map[string]func(args []string) (*Matcher, error){
	"call":              func(args []string) (*Matcher, error) { return callTo(args...) },
	"exported-func-doc": withoutArgs(exportedFuncDoc),
	"func-prefix":       func(args []string) (*Matcher, error) { return funcPrefix(args...) },
	"test-func":         withoutArgs(testFunc),
}

func withoutArgs(m *Matcher) func([]string) (*Matcher, error) {
	return func(args []string) (*Matcher, error) {
		if len(args) > 0 {
			return nil, fmt.Errorf("%s takes no arguments", m.Name)
		}

		return m, nil
	}
}

// matcher builds the matcher that spec describes.
func (spec MatchSpec) matcher() (*Matcher, error) {
	build, ok := matcherRegistry[spec.Matcher]
	if !ok {
		known := strings.Join(slices.Sorted(maps.Keys(matcherRegistry)), ", ")

		return nil, fmt.Errorf("unknown matcher %q (the matchers are: %s)", spec.Matcher, known)
	}

	return build(spec.Args)
}

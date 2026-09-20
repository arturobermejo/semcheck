package semcheck

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// matcherRegistry maps the names users write in a "match" field to the
// functions that build the matchers.
var matcherRegistry = map[string]func(args []string) (*matcher, error){
	"call":              func(args []string) (*matcher, error) { return callTo(args...) },
	"exported-func-doc": withoutArgs(exportedFuncDoc),
	"func-prefix":       func(args []string) (*matcher, error) { return funcPrefix(args...) },
	"test-func":         withoutArgs(testFunc),
}

func withoutArgs(m *matcher) func([]string) (*matcher, error) {
	return func(args []string) (*matcher, error) {
		if len(args) > 0 {
			return nil, fmt.Errorf("%s takes no arguments", m.Name)
		}

		return m, nil
	}
}

// matcher builds the matcher that spec describes.
func (spec MatchSpec) matcher() (*matcher, error) {
	build, ok := matcherRegistry[spec.Matcher]
	if !ok {
		known := strings.Join(slices.Sorted(maps.Keys(matcherRegistry)), ", ")

		return nil, fmt.Errorf("unknown matcher %q (the matchers are: %s)", spec.Matcher, known)
	}

	return build(spec.Args)
}

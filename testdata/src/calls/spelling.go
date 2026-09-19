package calls

import (
	l "log"

	applog "calls/internal/log"
)

type fakeLogger struct{}

func (fakeLogger) Printf(format string, args ...any) {}

// The package is resolved through the types, not through the text.
func spelling(u User) {
	l.Printf("renamed import: %v", u) // want "call: matched"

	// A variable named log whose type is declared in this package.
	log := fakeLogger{}
	log.Printf("not the log package: %v", u)

	// A package of ours named log: the pattern log.* gives its name.
	applog.Audit("login", u.Email) // want "call: matched"

	// A function value: there is no statically known callee.
	printf := l.Printf
	printf("through a variable: %v", u)
}

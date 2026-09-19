package funcs

import "sort"

// Function literals are expressions: they can appear anywhere an expression
// can, at any depth. None of them is in File.Decls.

var f = func() {} // want "found function literal"

func outer(names []string) { // want "found function outer"
	inner := func() {} // want "found function literal"
	inner()

	// As an argument.
	sort.Slice(names, func(i, j int) bool { // want "found function literal"
		return names[i] < names[j]
	})

	// Called in place.
	defer func() {}() // want "found function literal"

	go func() { // want "found function literal"
		// Nested: a literal inside a literal inside a declaration.
		_ = func() {} // want "found function literal"
	}()

	// Two on the same line: one want with two patterns.
	func() { func() {}() }() // want "found function literal" "found function literal"
}

// In a struct field and in a composite literal.
type handler struct {
	run func() // a function type, not a literal
}

var handlers = []handler{
	{run: func() {}}, // want "found function literal"
}

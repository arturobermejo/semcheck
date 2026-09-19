// Package exporteddoc exercises the exported-func-doc matcher.
package exporteddoc

// Documented is exported and has a doc comment.
func Documented() {} // want "exported-func-doc: matched"

func Undocumented() {}

// unexported has a doc comment but is not part of the API.
func unexported() {}

/*
BlockComment is documented with a block comment.
*/
func BlockComment() {} // want "exported-func-doc: matched"

// A blank line detaches a comment from the declaration: this is not a doc
// comment.

func Detached() {}

func Trailing() {} // Trailing comments are not doc comments either.

// Literal is a variable: only function declarations are selected.
var Literal = func() {}

// main is not exported, whatever it does.
func main() {}

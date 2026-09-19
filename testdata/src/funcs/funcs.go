// Package funcs exercises the laboratory analyzer: every function and method
// declaration and every function literal must be reported, and nothing else.
package funcs

import "fmt"

const c = 1

var v = fmt.Sprint(c)

type T struct{}

func Hello() {} // want "found function Hello"

func (t T) Method() {} // want `found function Method`

// A function type is not a function.
var callback func(int) error

// Declarations without a body are valid Go: the body lives in assembly.
func implementedElsewhere(x int) int // want "found function implementedElsewhere"

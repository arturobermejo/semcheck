// Package nofuncs has no functions of any kind: the analyzer must stay silent.
package nofuncs

const Answer = 42

// Transform is a function type, not a function.
type Transform func(int) int

// Identity has a function type, but no function literal as its value.
var Identity Transform

type Celsius float64

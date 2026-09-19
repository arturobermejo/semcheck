// Package nofuncs has no function declarations: the analyzer must stay silent.
package nofuncs

const Answer = 42

var Double = func(x int) int { return 2 * x }

type Celsius float64

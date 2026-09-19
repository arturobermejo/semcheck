// Package hello is a fixture with two function declarations and a literal.
package hello

// Hello greets.
func Hello() string { return "hi" }

type greeter struct{}

func (greeter) greet() string { return Hello() }

var shout = func() string { return "HI" }

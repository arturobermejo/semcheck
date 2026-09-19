// Package hello is a fixture with two function declarations.
package hello

// Hello greets.
func Hello() string { return "hi" }

type greeter struct{}

func (greeter) greet() string { return Hello() }

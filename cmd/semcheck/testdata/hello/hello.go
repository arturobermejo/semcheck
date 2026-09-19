// Package hello is a fixture with two documented exported functions.
package hello

// Hello greets.
func Hello() string { return "hi" }

type Greeter struct{}

// Greet greets too.
func (Greeter) Greet() string { return Hello() }

func Shout() string { return "HI" }

// whisper is documented but not exported.
func whisper() string { return "hi" }

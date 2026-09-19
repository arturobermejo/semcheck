// Package hello is a fixture with nodes for two matchers.
package hello

// Hello greets.
func Hello() string { return "hi" }

type Greeter struct{}

// Greet greets too.
func (Greeter) Greet() string { return Hello() }

func (Greeter) IsPolite() bool { return true }

// HasName is selected by both matchers.
func (Greeter) HasName() bool { return false }

func Shout() string { return "HI" }

// whisper is documented but not exported.
func whisper() string { return "hi" }

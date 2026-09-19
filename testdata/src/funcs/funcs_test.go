package funcs

import "testing"

// Test files are part of the package too.
func TestHello(t *testing.T) { // want "found function TestHello"
	Hello()
}

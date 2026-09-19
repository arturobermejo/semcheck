package exporteddoc

import "testing"

// TestDocumented is exported and documented like any other function: whether
// a rule applies to test files is for its configuration to decide.
func TestDocumented(t *testing.T) { // want "exported-func-doc: matched"
	Documented()
}

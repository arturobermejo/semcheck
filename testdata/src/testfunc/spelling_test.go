package testfunc

import (
	tt "testing"
)

// The type is testing.T, however the import is spelled.
func TestRenamedImport(t *tt.T) {} // want "test-func: matched"

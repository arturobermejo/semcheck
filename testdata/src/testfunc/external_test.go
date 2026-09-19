package testfunc_test

import "testing"

// An external test package is analyzed as a package of its own.
func TestExternal(t *testing.T) {} // want "test-func: matched"

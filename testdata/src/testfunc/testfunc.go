// Package testfunc exercises the test-func matcher.
package testfunc

import "testing"

// Right name and signature, wrong file: "go test" never runs it.
func TestInProductionFile(t *testing.T) {}

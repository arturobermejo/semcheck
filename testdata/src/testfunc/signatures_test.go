package testfunc

import "testing"

func TestMain(m *testing.M) {}

func BenchmarkLogin(b *testing.B) {}

func FuzzLogin(f *testing.F) {}

func ExampleLogin() {}

type suite struct{}

// Methods are not run by "go test", whatever a framework does with them.
func (s *suite) TestLogin(t *testing.T) {}

var TestVariable = func(t *testing.T) {}

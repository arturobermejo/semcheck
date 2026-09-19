package testfunc

import "testing"

func TestLogin(t *testing.T) {} // want "test-func: matched"

func Test(t *testing.T) {} // want "test-func: matched"

func Test_login(t *testing.T) {} // want "test-func: matched"

func Test2FA(t *testing.T) {} // want "test-func: matched"

// "go test" ignores names where a lowercase letter follows Test.
func Testify(t *testing.T) {}

func testLogin(t *testing.T) {}

func LoginTest(t *testing.T) {}

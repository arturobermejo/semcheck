// Package funcprefix exercises the func-prefix matcher with the prefixes Get,
// Is and Has.
package funcprefix

func GetUser() {} // want "func-prefix: matched"

func IsValid() bool { return true } // want "func-prefix: matched"

func HasPrefix() bool { return true } // want "func-prefix: matched"

// The prefix alone is a whole word.
func Get() {} // want "func-prefix: matched"

// Unexported names follow the same convention.
func getUser() {} // want "func-prefix: matched"

func isValid() bool { return true } // want "func-prefix: matched"

// A digit or an underscore also ends the word.
func Get2FA() {} // want "func-prefix: matched"

func Is_valid() bool { return true } // want "func-prefix: matched"

// These names start with the letters of a prefix, not with the word.
func Getter() {}

func Issue() {}

func Hash() {}

func island() {}

func gettysburg() {}

// The prefix must be at the start.
func ForgetUser() {}

func MustGetUser() {}

// Find and List are not among the prefixes of this test.
func FindUser() {}

func ListUsers() {}

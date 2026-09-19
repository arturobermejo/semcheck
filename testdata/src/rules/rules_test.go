package rules

import (
	"log"
	"testing"
)

func TestLogin(t *testing.T) { // want `^test-name-matches: the name does not say what the test checks \(0\.93\)$`
	u := User{Email: "a@example.com"}

	// Rules leave test files alone unless they say otherwise.
	log.Printf("created %s", u.Email)

	if err := Save(u); err != nil {
		t.Fatal(err)
	}
}

func TestSave(t *testing.T) {
	if err := Save(User{}); err != nil {
		t.Fatal(err)
	}
}

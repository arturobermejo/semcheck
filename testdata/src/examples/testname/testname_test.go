package testname

import "testing"

// want +2 `test-name-matches: `

func TestLoginRejectsAWrongPassword(t *testing.T) {
	if err := Login("ana", "secret"); err != nil {
		t.Fatal(err)
	}
}

// want +2 `test-name-matches: `

func TestParseReturnsAnErrorForEmptyInput(t *testing.T) {
	n, err := Parse("42")
	if err != nil || n != 42 {
		t.Fatalf("Parse = %d, %v", n, err)
	}
}

// want +2 `test-name-matches: `

func TestRemoveDeletesTheName(t *testing.T) {
	var s Store

	s.Add("ana")
	s.Add("luis")

	if s.Len() != 2 {
		t.Fatalf("Len = %d, want 2", s.Len())
	}
}

func TestLoginAcceptsTheRightPassword(t *testing.T) {
	if err := Login("ana", "secret"); err != nil {
		t.Fatal(err)
	}
}

func TestLoginRejectsAnUnknownUser(t *testing.T) {
	if err := Login("eva", "secret"); err == nil {
		t.Fatal("want an error")
	}
}

func TestParseEmptyInput(t *testing.T) {
	if _, err := Parse(""); err == nil {
		t.Fatal("want an error")
	}
}

func TestParse(t *testing.T) {
	n, err := Parse("42")
	if err != nil || n != 42 {
		t.Fatalf("Parse = %d, %v", n, err)
	}
}

func TestRemove(t *testing.T) {
	var s Store

	s.Add("ana")
	s.Remove("ana")

	if s.Len() != 0 {
		t.Fatalf("Len = %d, want 0", s.Len())
	}
}

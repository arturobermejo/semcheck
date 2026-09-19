// Package nolint is a fixture for //nolint directives. Every log is a finding
// unless a directive silences it; its message says which case it is.
package nolint

import "log"

func trailing() {
	log.Println("A reported")
	log.Println("B on the same line") //nolint:semcheck
	log.Println("C after a directive that ends the line above")
}

func ownLine() {
	//nolint:semcheck
	log.Println("D below a directive on its own line")
	log.Println("E the statement after that one")
}

//nolint:semcheck
func wholeFunction() {
	log.Println("F inside a function with the directive above it")
}

// docComment has the directive at the end of its doc comment.
//
//nolint:semcheck
func docComment() {
	log.Println("G inside a function with the directive in its doc comment")
}

func blocks(ok bool) {
	if ok { //nolint:semcheck
		log.Println("H inside a block whose first line has the directive")
	}

	log.Println("I a call that spans lines",
		1, 2) //nolint:semcheck

	//nolint:semcheck
	if ok {
		log.Println("J inside a block with the directive above it")
	}
}

func spelling() {
	log.Println("K bare") //nolint
	log.Println("L all") //nolint:all
	log.Println("M a list") //nolint:errcheck,semcheck
	log.Println("N with a reason") //nolint:semcheck // the ID is not personal data
	log.Println("O another linter") //nolint:errcheck
	log.Println("P a space after the slashes") // nolint:semcheck
	log.Println("Q upper case") //nolint:SEMCHECK
	log.Println("R spaces in the list") //nolint:errcheck, semcheck
	log.Println("S not a directive") // see nolint:semcheck in the docs
}

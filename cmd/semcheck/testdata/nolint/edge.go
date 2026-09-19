package nolint

import (
	"log"
	"net/http"
)

func edges() {
	//nolint:semcheck

	log.Println("U below a directive and a blank line")

	//nolint:semcheck
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println("V inside a closure of a statement with the directive above it")
	})

	/* nolint:semcheck */
	log.Println("W below a block comment")

	//nolint:semcheck // the reason
	//
	// and more text in the same comment group
	log.Println("X below a comment group that starts with the directive")
}

// second is documented.
//nolint:semcheck
// The directive is in the middle of the doc comment.
func middleOfDoc() {
	log.Println("Y inside a function with the directive in the middle of its doc")
}

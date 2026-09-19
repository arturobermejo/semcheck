package calls

import (
	"log"
	"net/http"
)

var _ = func() int { log.Println("package level literal"); return 0 }() // want "call: matched"

func places(u User) {
	defer log.Println("deferred", u.ID) // want "call: matched"

	go log.Println("goroutine", u.ID) // want "call: matched"

	if u.ID == 0 {
		for range 3 {
			log.Println("deep", u.Email) // want "call: matched"
		}
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println("in a closure", u.Email) // want "call: matched"
	})

	// A literal passed to a matching call starts over: both are selected.
	log.Println(func() string { // want "call: matched"
		log.Println("inside the argument") // want "call: matched"

		return "outer"
	}())
}

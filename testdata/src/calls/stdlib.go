package calls

import (
	"fmt"
	"log"
	"log/slog"
	"os"
)

// The four logs of the flagship rule. The matcher selects all of them; telling
// which ones leak personal data is the model's job.
func create(u User, fullName string) {
	slog.Info("user created", "email", u.Email)     // want "call: matched"
	slog.Info("user created", "contact", u.Contact) // want "call: matched"
	log.Printf("created %+v", u)                    // want "call: matched"
	log.Printf("welcome, %s", fullName)             // want "call: matched"
}

// Methods are matched by the package that declares them.
func methods(u User) {
	logger := slog.Default() // want "call: matched"
	logger.Info("user", "id", u.ID) // want "call: matched"

	std := log.New(os.Stderr, "", 0) // want "call: matched"
	std.Println(u.Email)             // want "call: matched"
}

// A matching call inside another one is part of it.
func nested(u User) {
	slog.Info("user", slog.String("email", u.Email)) // want "call: matched"

	slog.Default().With("id", u.ID).Info("user") // want "call: matched"
}

// Other packages are not selected, whatever their functions are called.
func others(u User) {
	fmt.Printf("created %+v\n", u)
	fmt.Println(fmt.Sprint(u))

	_ = len(u.Email)
	_ = string(rune(u.ID))
}

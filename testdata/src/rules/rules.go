// Package rules exercises the whole analyzer with the rules of
// testdata/config/rules.yml and the answers of a FakeJudge.
package rules

import (
	"log"
	"log/slog"
)

type User struct {
	ID    int
	Email string
}

func create(u User, fullName string) {
	// Above the threshold of the rule.
	slog.Info("user created", "email", u.Email) // want `^no-pii-in-logs: this log includes personal data \(0\.97\)$`

	// Below it: 0.85 is not enough for a rule that asks for 0.9.
	log.Printf("welcome, %s", fullName)

	log.Printf("created user %d", u.ID)

	// Two rules can find something on the same line.
	slog.Info("cannot reach the database", "email", u.Email) // want `^log-level-fits: the level does not fit the message \(0\.88\)$` `^no-pii-in-logs: .* \(0\.97\)$`
}

// Save stores the user.
func Save(u User) error { // want `^doc-matches-code: the answer to "Does the comment describe what the function does\?" is no \(0\.96\)$`
	return remove(u.ID)
}

// Find returns the user with the given ID.
func Find(id int) User { return User{ID: id} }

func remove(id int) error { return nil }

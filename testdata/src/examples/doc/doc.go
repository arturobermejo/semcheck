// Package doc has what the rule doc-matches-code must and must not report.
package doc

import (
	"errors"
	"log"
	"strings"
)

type User struct {
	ID     int
	Name   string
	Active bool
}

type Store struct {
	users map[int]User
}

// want +3 `doc-matches-code: `

// Sum returns the sum of the numbers.
func Sum(numbers []int) int {
	total := 1
	for _, n := range numbers {
		total *= n
	}

	return total
}

// want +3 `doc-matches-code: `

// Add stores the user. It returns an error if there is one with that ID already.
func (s *Store) Add(u User) error {
	s.users[u.ID] = u

	return nil
}

// want +3 `doc-matches-code: `

// Names returns the names of the active users, sorted.
func (s *Store) Names() []string {
	var names []string

	for _, u := range s.users {
		names = append(names, u.Name)
	}

	return names
}

// want +3 `doc-matches-code: `

// Normalize returns the name in lower case. It never changes its argument.
func Normalize(names []string) []string {
	for i, name := range names {
		names[i] = strings.ToLower(name)
	}

	return names
}

// Total returns the sum of the numbers.
func Total(numbers []int) int {
	total := 0
	for _, n := range numbers {
		total += n
	}

	return total
}

// Len returns the number of users.
func (s *Store) Len() int { return len(s.users) }

// Save stores the user.
func (s *Store) Save(u User) {
	s.users[u.ID] = u
	log.Printf("saved user %d", u.ID)
}

// Find returns the user with the given ID, or an error if there is none.
func (s *Store) Find(id int) (User, error) {
	u, ok := s.users[id]
	if !ok {
		return User{}, errors.New("no such user")
	}

	return u, nil
}

// Deactivate marks the user as inactive. It does nothing if there is no such
// user.
func (s *Store) Deactivate(id int) {
	if u, ok := s.users[id]; ok {
		u.Active = false
		s.users[id] = u
	}
}

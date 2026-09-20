// Package testname has what the rule test-name-matches must and must not
// report. The tests are in testname_test.go.
package testname

import (
	"errors"
	"strconv"
)

var passwords = map[string]string{"ana": "secret"}

func Login(user, password string) error {
	if passwords[user] != password {
		return errors.New("wrong user or password")
	}

	return nil
}

func Parse(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty input")
	}

	return strconv.Atoi(s)
}

type Store struct{ names []string }

func (s *Store) Add(name string) { s.names = append(s.names, name) }

func (s *Store) Remove(name string) {
	for i, n := range s.names {
		if n == name {
			s.names = append(s.names[:i], s.names[i+1:]...)

			return
		}
	}
}

func (s *Store) Len() int { return len(s.names) }

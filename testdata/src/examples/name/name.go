// Package name has what the rule name-matches-behavior must and must not
// report.
package name

import "time"

type User struct {
	ID     int
	Name   string
	Email  string
	Active bool
	seen   time.Time
}

var (
	users  = map[int]*User{}
	visits int
)

// want +2 `name-matches-behavior: `

func GetUser(id int) *User {
	u := users[id]
	if u == nil {
		u = &User{ID: id}
		users[id] = u
	}

	return u
}

// want +2 `name-matches-behavior: `

func IsExpired(u *User) bool {
	if time.Since(u.seen) > time.Hour {
		delete(users, u.ID)

		return true
	}

	return false
}

// want +2 `name-matches-behavior: `

func HasAccess(u *User) bool {
	visits++
	u.seen = time.Now()

	return u.Active
}

// want +2 `name-matches-behavior: `

func FindByEmail(email string) *User {
	for _, u := range users {
		if u.Email == email {
			u.seen = time.Now()

			return u
		}
	}

	return nil
}

// want +2 `name-matches-behavior: `

func CountActive() int {
	n := 0

	for id, u := range users {
		if !u.Active {
			delete(users, id)

			continue
		}

		n++
	}

	return n
}

func GetName(u *User) string { return u.Name }

func IsEmpty() bool { return len(users) == 0 }

func HasEmail(u *User) bool { return u.Email != "" }

func FindByName(name string) *User {
	for _, u := range users {
		if u.Name == name {
			return u
		}
	}

	return nil
}

func ListNames() []string {
	names := make([]string, 0, len(users))
	for _, u := range users {
		names = append(names, u.Name)
	}

	return names
}

func CountVisits() int { return visits }

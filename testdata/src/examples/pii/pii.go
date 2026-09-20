// Package pii has what the rule no-pii-in-logs must and must not report.
package pii

import (
	"errors"
	"log"
	"log/slog"
	"time"

	zlog "github.com/rs/zerolog/log"
	"go.uber.org/zap"
)

type Contact struct {
	Email string
	Phone string
}

type User struct {
	ID      int
	Name    string
	Contact Contact
	seen    time.Time
}

type Order struct {
	ID    string
	Total int
}

type Settings struct {
	Port  int
	Debug bool
}

func personalData(u *User, fullName, email string, logger *zap.Logger) {
	log.Printf("saved user %+v", u)                                 // want `no-pii-in-logs: `
	log.Printf("new signup: %s", u.Contact.Email)                   // want `no-pii-in-logs: `
	log.Println("calling", u.Contact.Phone)                         // want `no-pii-in-logs: `
	log.Printf("welcome, %s", fullName)                             // want `no-pii-in-logs: `
	log.Printf("password reset requested for %s", email)            // want `no-pii-in-logs: `
	slog.Info("user created", "name", u.Name, "id", u.ID)           // want `no-pii-in-logs: `
	logger.Info("signup", zap.String("email", u.Contact.Email))     // want `no-pii-in-logs: `
	zlog.Info().Str("phone", u.Contact.Phone).Msg("calling a user") // want `no-pii-in-logs: `
}

func nothingPersonal(u *User, o Order, s Settings, n int, logger *zap.Logger) {
	log.Printf("saved user %d", u.ID)
	log.Printf("counting %d users", n)
	log.Println("server started")
	log.Printf("request failed: %v", errors.New("timeout"))
	log.Printf("order %s shipped, total %d", o.ID, o.Total)
	log.Printf("loaded settings %+v", s)
	log.Printf("cache refreshed in %s", time.Since(u.seen))
	slog.Info("user created", "id", u.ID)
	logger.Info("order shipped", zap.String("order", o.ID))
	zlog.Info().Int("users", n).Msg("counted the users")
}

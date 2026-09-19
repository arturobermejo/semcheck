// Package calls exercises the call matcher with the scenario of the
// no-pii-in-logs rule. The patterns are log.*, slog.*, zap.* and zerolog.*.
package calls

type Contact struct {
	Email string
	Phone string
}

type User struct {
	ID      int
	Email   string
	Contact Contact
}

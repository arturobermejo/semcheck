// Package log is a minimal double of github.com/rs/zerolog/log. Its name
// collides with the standard library's log on purpose: so does the real one.
package log

import "github.com/rs/zerolog"

func Info() *zerolog.Event { return &zerolog.Event{} }

func Error() *zerolog.Event { return &zerolog.Event{} }

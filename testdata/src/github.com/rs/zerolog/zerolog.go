// Package zerolog is a minimal double of github.com/rs/zerolog.
package zerolog

type Event struct{}

func (e *Event) Str(key, val string) *Event { return e }

func (e *Event) Interface(key string, val any) *Event { return e }

func (e *Event) Msg(msg string) {}

func (e *Event) Send() {}

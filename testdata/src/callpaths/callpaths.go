// Package callpaths exercises call patterns that give the import path, which
// tells apart packages that share a name.
package callpaths

import (
	"log"
	"log/slog"

	zlog "github.com/rs/zerolog/log"
)

func paths() {
	zlog.Info().Msg("zerolog's log package") // want "call: matched"

	log.Printf("the standard library's log package")

	slog.Info("a single function") // want "call: matched"
	slog.Error("another function of the same package")
}

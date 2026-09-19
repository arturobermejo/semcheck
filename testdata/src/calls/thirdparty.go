package calls

import (
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"go.uber.org/zap"
)

func thirdParty(u User, logger *zap.Logger) {
	logger.Info("user", zap.String("email", u.Email)) // want "call: matched"
	zap.L().Info("user", zap.Any("user", u))          // want "call: matched"

	// A fluent chain is three calls and one statement.
	zlog.Info().Str("email", u.Email).Msg("user created") // want "call: matched"

	zlog.Error(). // want "call: matched"
			Interface("user", u).
			Send()

	// A chain that is kept in a variable is two statements.
	var ev *zerolog.Event = zlog.Info() // want "call: matched"
	ev.Str("email", u.Email).Send()     // want "call: matched"
}

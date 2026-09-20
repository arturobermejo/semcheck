// Package level has what the rule log-level-fits must and must not report.
package level

import (
	"log/slog"

	zlog "github.com/rs/zerolog/log"
	"go.uber.org/zap"
)

func wrongLevel(err error, port int, logger *zap.Logger) {
	slog.Info("failed to connect to the database", "err", err)      // want `log-level-fits: `
	slog.Debug("could not save the order", "err", err)              // want `log-level-fits: `
	slog.Error("server started", "port", port)                      // want `log-level-fits: `
	logger.Info("could not write the file", zap.Error(err))         // want `log-level-fits: `
	zlog.Info().Err(err).Msg("lost the connection to the database") // want `log-level-fits: `
	zlog.Error().Int("port", port).Msg("listening")                 // want `log-level-fits: `
}

func rightLevel(err error, port, attempt int, key string, logger *zap.Logger) {
	slog.Error("failed to connect to the database", "err", err)
	slog.Info("server started", "port", port)
	slog.Debug("cache miss", "key", key)
	slog.Warn("retrying the request", "attempt", attempt)
	logger.Error("could not write the file", zap.Error(err))
	logger.Debug("parsed the request", zap.Int("fields", attempt))
	zlog.Error().Err(err).Msg("lost the connection to the database")
	zlog.Info().Int("port", port).Msg("listening")
}

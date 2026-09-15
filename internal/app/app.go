package app

import (
	"io"
	"log/slog"

	"github.com/mananuf/justbarme/internal/config"
)

var Version = "dev"

func newLogger(cfg config.Config, output io.Writer) *slog.Logger {
	level := new(slog.LevelVar)
	switch cfg.LogLevel {
	case "debug":
		level.Set(slog.LevelDebug)
	case "warn":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelInfo)
	}

	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level}))
}

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mananuf/justbarme/internal/app"
)

func main() {
	if err := app.LoadDotEnvIfPresent(); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

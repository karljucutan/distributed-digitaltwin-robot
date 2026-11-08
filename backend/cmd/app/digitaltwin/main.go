package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/karljucutan/distributed-digitaltwin-robot/internal/app/digitaltwin"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := digitaltwin.NewApp(ctx)
	if err != nil {
		slog.Error("initialize application", "error", err)
		stop()
		os.Exit(1)
	}

	if err := app.Run(ctx); err != nil {
		slog.Error("application failed", "error", err)
		stop()
		os.Exit(1)
	}
}

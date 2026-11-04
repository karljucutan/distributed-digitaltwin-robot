package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/karljucutan/distributed-digitaltwin-robot/pkg/configuration"
)

type App struct {
	apiServer *http.Server
}

func NewApp() *App {

	return &App{
		apiServer: &http.Server{
			Addr:              ":" + configuration.GetEnv("API_PORT", "8080"),
			ReadHeaderTimeout: time.Duration(configuration.GetEnvAsInt("API_READ_HEADER_TIMEOUT", 5)) * time.Second,
			ReadTimeout:       time.Duration(configuration.GetEnvAsInt("API_READ_TIMEOUT", 10)) * time.Second,
			WriteTimeout:      time.Duration(configuration.GetEnvAsInt("API_WRITE_TIMEOUT", 30)) * time.Second,
			IdleTimeout:       time.Duration(configuration.GetEnvAsInt("API_IDLE_TIMEOUT", 60)) * time.Second,
			Handler:           newHttpHandler(),
		},
	}
}

func (a *App) Run(ctx context.Context) error {
	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("starting API server", "addr", a.apiServer.Addr)
		serverErrors <- a.apiServer.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve API: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.apiServer.Shutdown(shutdownCtx); err != nil {
		_ = a.apiServer.Close()
		return fmt.Errorf("shutdown API server: %w", err)
	}

	if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve API: %w", err)
	}

	slog.Info("API server stopped")
	return nil
}

func newHttpHandler() http.Handler {
	httpMux := http.NewServeMux()
	// register routes
	return httpMux
}

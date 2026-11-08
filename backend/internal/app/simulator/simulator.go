package simulator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/karljucutan/distributed-digitaltwin-robot/internal/features/fleetsimulator"
	"github.com/karljucutan/distributed-digitaltwin-robot/pkg/configuration"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type App struct {
	apiServer *http.Server
	nc        *nats.Conn
}

func NewApp(appCtx context.Context) (*App, error) {
	nc, err := nats.Connect(configuration.GetEnv("NATS_URL", nats.DefaultURL))
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create JetStream client: %w", err)
	}

	streamCtx, cancel := context.WithTimeout(appCtx, 10*time.Second)
	defer cancel()

	// Option: Provision this stream through deployment infrastructure in production.
	if _, err := js.CreateOrUpdateStream(streamCtx, jetstream.StreamConfig{
		Name:     "ROBOT_TELEMETRY",
		Subjects: []string{"telemetry.robot.*"},
		Storage:  jetstream.FileStorage,
		MaxAge:   time.Hour,
		MaxBytes: 1 << 30,
	}); err != nil {
		nc.Close()
		return nil, fmt.Errorf("configure robot telemetry stream: %w", err)
	}

	manager := &fleetsimulator.SimulationManager{
		JetStream: js,
	}

	return &App{
		apiServer: &http.Server{
			Addr:              ":" + configuration.GetEnv("API_PORT", "8080"),
			ReadHeaderTimeout: time.Duration(configuration.GetEnvAsInt("API_READ_HEADER_TIMEOUT", 5)) * time.Second,
			ReadTimeout:       time.Duration(configuration.GetEnvAsInt("API_READ_TIMEOUT", 10)) * time.Second,
			WriteTimeout:      time.Duration(configuration.GetEnvAsInt("API_WRITE_TIMEOUT", 30)) * time.Second,
			IdleTimeout:       time.Duration(configuration.GetEnvAsInt("API_IDLE_TIMEOUT", 60)) * time.Second,
			Handler:           newHttpServer(appCtx, manager),
		},
		nc: nc,
	}, nil
}

func (a *App) Run(appCtx context.Context) error {
	defer func() {
		closed := a.nc.StatusChanged(nats.CLOSED)
		defer a.nc.RemoveStatusListener(closed)

		if err := a.nc.Drain(); err != nil {
			slog.Error("drain NATS", "error", err)
			return
		}
		<-closed
	}()

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
	case <-appCtx.Done():
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

func newHttpServer(
	appCtx context.Context,
	simulationManager *fleetsimulator.SimulationManager) http.Handler {
	httpMux := http.NewServeMux()

	httpMux.HandleFunc("POST /api/simulation", func(w http.ResponseWriter, r *http.Request) {
		fleetsimulator.SimulationHttpHandler(appCtx, simulationManager, w, r)
	})

	return httpMux
}

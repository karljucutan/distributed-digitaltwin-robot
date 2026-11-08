package digitaltwin

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/karljucutan/distributed-digitaltwin-robot/internal/features/digitaltwin"
	"github.com/karljucutan/distributed-digitaltwin-robot/pkg/configuration"
	"github.com/karljucutan/distributed-digitaltwin-robot/pkg/retry"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type App struct {
	nc  *nats.Conn
	dte *digitaltwin.Engine
}

func NewApp(appCtx context.Context) (*App, error) {
	startupCtx, cancel := context.WithTimeout(appCtx, 30*time.Second)
	defer cancel()

	nc, err := nats.Connect(configuration.GetEnv("NATS_URL", nats.DefaultURL))
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create JetStream client: %w", err)
	}

	// cons, err := js.CreateOrUpdateConsumer(appCtx, "ROBOT_TELEMETRY", jetstream.ConsumerConfig{
	// 	Durable:       "digital-twin", // consumer in NATS Server. Consumer: digital-twin
	// 	DeliverPolicy: jetstream.DeliverAllPolicy,
	// 	AckPolicy:     jetstream.AckExplicitPolicy,
	// })
	// if err != nil {
	// 	nc.Close()
	// 	return nil, fmt.Errorf("create or update consumer: %w", err)
	// }

	var cons jetstream.Consumer
	err = retry.Do(startupCtx, "create or update consumer", func(ctx context.Context) error {
		var err error
		cons, err = js.CreateOrUpdateConsumer(ctx, "ROBOT_TELEMETRY", jetstream.ConsumerConfig{
			Durable:       "digital-twin",
			DeliverPolicy: jetstream.DeliverAllPolicy,
			AckPolicy:     jetstream.AckExplicitPolicy,
		})
		return err
	})
	if err != nil {
		nc.Close()
		return nil, err
	}

	dtEngine := digitaltwin.NewEngine(cons)

	return &App{
		nc:  nc,
		dte: dtEngine,
	}, nil
}

func (a *App) Run(appCtx context.Context) error {
	defer func() {
		closed := a.nc.StatusChanged(nats.CLOSED)
		defer a.nc.RemoveStatusListener(closed)

		if err := a.nc.Drain(); err != nil {
			slog.Error("drain NATS", "error", err)
		}
	}()

	slog.Info("starting digital twin engine")

	if err := a.dte.Run(appCtx); err != nil {
		return fmt.Errorf("run digital twin engine: %w", err)
	}

	slog.Info("digital twin engine stopped")

	return nil
}

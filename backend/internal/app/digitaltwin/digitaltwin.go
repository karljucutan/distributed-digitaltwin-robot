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
	"github.com/valkey-io/valkey-go"
)

type App struct {
	natsConn          *nats.Conn
	digitalTwinEngine *digitaltwin.Engine
	valkeyClient      valkey.Client
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

	// Connect to a single valkey node:
	valkeyClient, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{"valkey:6379"},
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create valkey client: %w", err)
	}

	dtEngine := digitaltwin.NewEngine(cons, valkeyClient)

	return &App{
		natsConn:          nc,
		digitalTwinEngine: dtEngine,
		valkeyClient:      valkeyClient,
	}, nil
}

func (a *App) Run(appCtx context.Context) error {
	defer func() {
		closed := a.natsConn.StatusChanged(nats.CLOSED)
		defer a.natsConn.RemoveStatusListener(closed)

		if err := a.natsConn.Drain(); err != nil {
			slog.Error("drain NATS", "error", err)
		}
	}()

	slog.Info("starting digital twin engine")

	if err := a.digitalTwinEngine.Run(appCtx); err != nil {
		return fmt.Errorf("run digital twin engine: %w", err)
	}

	slog.Info("digital twin engine stopped")

	return nil
}

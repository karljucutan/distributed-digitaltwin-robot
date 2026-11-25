package digitaltwin

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karljucutan/distributed-digitaltwin-robot/internal/features/digitaltwin"
	"github.com/karljucutan/distributed-digitaltwin-robot/pkg/configuration"
	"github.com/karljucutan/distributed-digitaltwin-robot/pkg/retry"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/valkey-io/valkey-go"
)

type App struct {
	dbPool               *pgxpool.Pool
	natsConn             *nats.Conn
	digitalTwinEngine    *digitaltwin.Engine
	valkeyClient         valkey.Client
	telemetryBatchWriter *digitaltwin.TelemetryBatchWriter
}

func NewApp(appCtx context.Context) (_ *App, err error) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// On failure, close resources acquired so far in reverse order.
	// On success, the returned App owns them.
	var closers []func()
	defer func() {
		if err != nil {
			for _, closer := range slices.Backward(closers) {
				closer()
			}
		}
	}()

	startupCtx, cancel := context.WithTimeout(appCtx, 30*time.Second)
	defer cancel()

	nc, err := nats.Connect(configuration.GetEnv("NATS_URL", nats.DefaultURL))
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}
	closers = append(closers, nc.Close)

	js, err := jetstream.New(nc)
	if err != nil {
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
			MaxAckPending: configuration.GetEnvAsInt("NATS_MAX_ACK_PENDING", 1000),
		})
		return err
	})
	if err != nil {
		return nil, err
	}

	// Connect to a single valkey node:
	valkeyClient, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{"valkey:6379"},
	})
	if err != nil {
		return nil, fmt.Errorf("create valkey client: %w", err)
	}
	closers = append(closers, valkeyClient.Close)

	dbPool, err := openDatabase(appCtx)
	if err != nil {
		return nil, err
	}
	closers = append(closers, dbPool.Close)

	telemetryRepository := digitaltwin.NewTelemetryRepository(dbPool)

	queueSize := configuration.GetEnvAsInt("TIMESCALEDB_WRITE_QUEUE_SIZE", 1000)
	batchSize := configuration.GetEnvAsInt("TIMESCALEDB_FLUSH_BATCH_SIZE", 5000)
	telemetryBatchWriter := digitaltwin.NewTelemetryBatchWriter(queueSize, batchSize, telemetryRepository)
	dtEngine := digitaltwin.NewEngine(cons, valkeyClient, telemetryBatchWriter)

	return &App{
		dbPool:               dbPool,
		natsConn:             nc,
		digitalTwinEngine:    dtEngine,
		valkeyClient:         valkeyClient,
		telemetryBatchWriter: telemetryBatchWriter,
	}, nil
}

func (a *App) Run(appCtx context.Context) error {
	defer a.dbPool.Close()
	defer a.valkeyClient.Close()
	defer func() {
		closed := a.natsConn.StatusChanged(nats.CLOSED)
		defer a.natsConn.RemoveStatusListener(closed)

		if err := a.natsConn.Drain(); err != nil {
			slog.Error("drain NATS", "error", err)
		}
	}()

	if err := a.runTelemetryBatchWriter(appCtx); err != nil {
		return fmt.Errorf("run telemetry batch writer: %w", err)
	}

	slog.Info("starting digital twin engine")

	if err := a.digitalTwinEngine.Run(appCtx); err != nil {
		return fmt.Errorf("run digital twin engine: %w", err)
	}

	slog.Info("digital twin engine stopped")

	return nil
}

func (a *App) runTelemetryBatchWriter(appCtx context.Context) error {
	flushInterval, err := time.ParseDuration(configuration.GetEnv("TIMESCALEDB_FLUSH_INTERVAL", "5s"))
	if err != nil {
		return fmt.Errorf("parse TIMESCALEDB_FLUSH_INTERVAL: %w", err)
	}
	if flushInterval <= 0 {
		return fmt.Errorf("TIMESCALEDB_FLUSH_INTERVAL must be positive, got %s", flushInterval)
	}

	slog.Info("starting telemetry batch writer with flush interval", "interval", flushInterval)
	a.telemetryBatchWriter.Run(appCtx, flushInterval)
	return nil
}

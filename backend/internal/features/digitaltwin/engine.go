package digitaltwin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/karljucutan/distributed-digitaltwin-robot/internal/domain"
	"github.com/karljucutan/distributed-digitaltwin-robot/pkg/configuration"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/valkey-io/valkey-go"
	"golang.org/x/sync/errgroup"
)

type Engine struct {
	jetstream    jetstream.Consumer
	valkeyClient valkey.Client
}

func NewEngine(consumer jetstream.Consumer, valkeyClient valkey.Client) *Engine {
	return &Engine{
		jetstream:    consumer,
		valkeyClient: valkeyClient,
	}
}

func (e *Engine) Run(appCtx context.Context) error {
	// NATS JS already has a built-in mechanism for client-side buffering.
	// I will use it instead of my initial plan to implement a local buffer using a channel.
	// 	robotTelemetries := make(chan *domain.RobotTelemetry, bufferSize) // Buffered channel for robot telemetry; sends block when full, slowing message intake.

	workerCount := configuration.GetEnvAsInt("ENGINE_WORKER_COUNT", 64)
	bufferSize := configuration.GetEnvAsInt("ENGINE_BUFFER_SIZE", 10000)
	messagesCtx, err := e.jetstream.Messages(jetstream.PullMaxMessages(bufferSize))
	if err != nil {
		return fmt.Errorf("failed to initialize NATS streaming iterator: %w", err)
	}
	defer messagesCtx.Stop()

	group, groupCtx := errgroup.WithContext(appCtx)
	for i := range workerCount {
		group.Go(func() error {
			slog.Info("worker started", "worker_id", i)
			defer slog.Info("worker stopped", "worker_id", i)

			for {
				msg, err := messagesCtx.Next(jetstream.NextContext(groupCtx))
				// Next can return a buffered message as cancellation happens.
				if groupCtx.Err() != nil {
					return nil
				}
				if err != nil {
					return fmt.Errorf("worker %d: get message: %w", i, err)
				}

				var telemetry domain.RobotTelemetry
				if err := json.Unmarshal(msg.Data(), &telemetry); err != nil {
					return fmt.Errorf("worker %d: unmarshal message: %w", i, err)
				}

				if err := processTelemetry(&telemetry); err != nil {
					return fmt.Errorf("worker %d: process telemetry: %w", i, err)
				}

				if err := msg.Ack(); err != nil {
					return fmt.Errorf("worker %d: acknowledge telemetry: %w", i, err)
				}
			}
		})
	}

	if err := group.Wait(); err != nil {
		return err
	}

	return nil
}

func processTelemetry(telemetry *domain.RobotTelemetry) error {
	// Implement the actual processing logic for the telemetry here.
	// For now, just print it as a placeholder.

	// add valkey
	// digital twin processing logic goes here

	// stage 2: this will be in another goroutine/worker that handles batch saving to TimescaleDB and acking messages
	// add timescaledb
	// add array buffer for batch saving to db

	slog.Info("Processing telemetry", "telemetry", telemetry)
	return nil
}

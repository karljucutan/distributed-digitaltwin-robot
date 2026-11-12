package digitaltwin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/karljucutan/distributed-digitaltwin-robot/internal/domain"
	"github.com/karljucutan/distributed-digitaltwin-robot/pkg/configuration"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/valkey-io/valkey-go"
	"golang.org/x/sync/errgroup"
)

const (
	twinUpdateTimeout = 2 * time.Second
	maxTwinRetries    = 8
)

type twinRepository interface {
	Get(ctx context.Context, robotID int64) (domain.RobotTwin, error)
	Save(ctx context.Context, twin domain.RobotTwin, expectedVersion uint64) error
}

type Engine struct {
	jetstream      jetstream.Consumer
	twinRepository twinRepository
}

func NewEngine(consumer jetstream.Consumer, valkeyClient valkey.Client) *Engine {
	return &Engine{
		jetstream:      consumer,
		twinRepository: newTwinStore(valkeyClient),
	}
}

func (e *Engine) Run(appCtx context.Context) error {
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

				if err := e.handleMessage(groupCtx, msg); err != nil {
					if groupCtx.Err() != nil {
						return nil // Shutting down; the unacked message is redelivered.
					}
					return fmt.Errorf("worker %d: %w", i, err)
				}
			}
		})
	}

	return group.Wait()
}

func (e *Engine) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	var telemetry domain.RobotTelemetry
	if err := json.Unmarshal(msg.Data(), &telemetry); err != nil {
		return rejectMessage(msg, "malformed telemetry", err)
	}

	metadata, err := msg.Metadata()
	if err != nil {
		return fmt.Errorf("read message metadata: %w", err)
	}

	err = e.processTelemetry(ctx, telemetry, metadata.Sequence.Stream)
	if errors.Is(err, domain.ErrInvalidTelemetry) {
		return rejectMessage(msg, "invalid telemetry", err)
	}
	if err != nil {
		return fmt.Errorf("process telemetry: %w", err)
	}

	if err := msg.DoubleAck(ctx); err != nil {
		return fmt.Errorf("acknowledge telemetry: %w", err)
	}
	return nil
}

func (e *Engine) processTelemetry(
	ctx context.Context,
	telemetry domain.RobotTelemetry,
	streamSequence uint64,
) error {
	if err := telemetry.Validate(); err != nil {
		return err
	}
	if streamSequence == 0 {
		return errors.New("process telemetry: JetStream stream sequence is zero")
	}

	ctx, cancel := context.WithTimeout(ctx, twinUpdateTimeout)
	defer cancel()

	for attempt := range maxTwinRetries {
		twin, err := e.twinRepository.Get(ctx, telemetry.RobotID)
		if err != nil {
			return err
		}

		expectedVersion := twin.Version
		if !twin.Apply(telemetry, streamSequence) {
			return nil // Duplicate or older telemetry.
		}

		err = e.twinRepository.Save(ctx, twin, expectedVersion)
		if !errors.Is(err, errTwinConflict) {
			return err
		}

		if err := waitBeforeRetry(ctx, attempt); err != nil {
			return fmt.Errorf("retry twin %d: %w", telemetry.RobotID, err)
		}
	}

	return fmt.Errorf("update twin %d: conflicted %d times", telemetry.RobotID, maxTwinRetries)
}

func rejectMessage(msg jetstream.Msg, reason string, cause error) error {
	slog.Error("rejecting telemetry", "reason", reason, "error", cause)
	if err := msg.TermWithReason(reason); err != nil {
		return fmt.Errorf("terminate message: %w", err)
	}
	return nil
}

func waitBeforeRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(attempt+1) * time.Millisecond)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

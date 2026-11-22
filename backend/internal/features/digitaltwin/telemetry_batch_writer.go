package digitaltwin

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/karljucutan/distributed-digitaltwin-robot/internal/domain"
)

type TelemetryBatchWriter struct {
	queue               chan pendingTelemetry
	batchSize           int
	telemetryRepository *TelemetryRepository
}

type pendingTelemetry struct {
	telemetry   domain.RobotTelemetry
	onCommitted func() error
}

func NewTelemetryBatchWriter(
	queueSize int,
	batchSize int,
	telemetryRepository *TelemetryRepository,
) *TelemetryBatchWriter {
	return &TelemetryBatchWriter{
		queue:               make(chan pendingTelemetry, queueSize),
		batchSize:           batchSize,
		telemetryRepository: telemetryRepository,
	}
}

// Enqueue queues telemetry and its callback for background batch processing.
// It blocks until queue space is available, not until the batch is flushed.
func (tbw *TelemetryBatchWriter) Enqueue(pendingTelemetry pendingTelemetry) {
	tbw.queue <- pendingTelemetry
}

func (tbw *TelemetryBatchWriter) Run(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		batch := make([]pendingTelemetry, 0, tbw.batchSize)

		for {
			select {
			case <-ctx.Done():
				var err error
				batch, err = tbw.flush(ctx, batch)
				if err != nil {
					slog.Error("failed to flush telemetry batch on shutdown", "error", err)
				}
				slog.Info("telemetry batch writer stopped", "reason", ctx.Err())
				return

			case pendingTelemetry, ok := <-tbw.queue:
				if !ok {
					return
				}

				batch = append(batch, pendingTelemetry)
				if len(batch) >= tbw.batchSize {
					var err error
					batch, err = tbw.flush(ctx, batch)
					if err != nil {
						slog.Error("failed to flush telemetry batch", "error", err)
					}
					ticker.Reset(interval)
				}

			case <-ticker.C:
				if len(batch) > 0 {
					var err error
					batch, err = tbw.flush(ctx, batch)
					if err != nil {
						slog.Error("failed to flush telemetry batch", "error", err)
					}
				}
			}
		}
	}()
}

func (tbw *TelemetryBatchWriter) flush(ctx context.Context, batch []pendingTelemetry) ([]pendingTelemetry, error) {
	telemetries := make([]domain.RobotTelemetry, len(batch))
	for i := range batch {
		telemetries[i] = batch[i].telemetry
	}
	if err := tbw.telemetryRepository.WriteBatch(ctx, telemetries); err != nil {
		return batch, fmt.Errorf("write telemetry batch: %w", err)
	}

	for _, pendingTelemetry := range batch {
		if pendingTelemetry.onCommitted != nil {
			if err := pendingTelemetry.onCommitted(); err != nil {
				slog.Error("post-commit callback failed", "error", err)
			}
		}
	}

	return batch[:0], nil
}

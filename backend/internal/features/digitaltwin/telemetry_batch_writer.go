package digitaltwin

import (
	"context"
	"log/slog"
	"time"

	"github.com/karljucutan/distributed-digitaltwin-robot/internal/domain"
)

type TelemetryBatchWriter struct {
	queue     chan domain.RobotTelemetry
	batchSize int
}

func NewTelemetryBatchWriter(queueSize int, batchSize int) *TelemetryBatchWriter {
	return &TelemetryBatchWriter{
		queue:     make(chan domain.RobotTelemetry, queueSize),
		batchSize: batchSize,
	}
}

func (tbw *TelemetryBatchWriter) Insert(telemetry domain.RobotTelemetry) {
	tbw.queue <- telemetry
}

func (tbw *TelemetryBatchWriter) Run(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		batch := make([]domain.RobotTelemetry, 0, tbw.batchSize)

		for {
			select {
			case <-ctx.Done():
				batch = tbw.flush(batch)
				slog.Info("telemetry batch writer stopped", "reason", ctx.Err())
				return

			case telemetry, ok := <-tbw.queue:
				if !ok {
					return
				}

				batch = append(batch, telemetry)
				if len(batch) >= tbw.batchSize {
					batch = tbw.flush(batch)
					ticker.Reset(interval)
				}

			case <-ticker.C:
				if len(batch) > 0 {
					batch = tbw.flush(batch)
				}
			}
		}
	}()
}

func (tbw *TelemetryBatchWriter) flush(batch []domain.RobotTelemetry) []domain.RobotTelemetry {
	// write to db
	slog.Info("flushing telemetry batch", "batch_size", len(batch))
	return batch[:0]
}

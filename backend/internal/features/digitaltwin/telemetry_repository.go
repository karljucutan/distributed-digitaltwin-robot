package digitaltwin

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karljucutan/distributed-digitaltwin-robot/internal/domain"
)

type TelemetryRepository struct {
	pool *pgxpool.Pool
}

func NewTelemetryRepository(pool *pgxpool.Pool) *TelemetryRepository {
	return &TelemetryRepository{pool: pool}
}

func (r *TelemetryRepository) WriteBatch(ctx context.Context, batch []domain.RobotTelemetry) error {
	if len(batch) == 0 {
		return nil
	}

	_, err := r.pool.CopyFrom(ctx, pgx.Identifier{"public", "robot_telemetry"},
		[]string{
			"timestamp", "robot_id", "sequence",
			"joint_position_rad", "joint_velocity_rad_per_sec", "status", "fault_code",
		},
		pgx.CopyFromSlice(len(batch), func(i int) ([]any, error) {
			t := &batch[i]
			return []any{
				t.Timestamp, t.RobotID, t.Sequence,
				t.JointPositionRad[:], t.JointVelocityRadPerSec[:],
				string(t.Status), t.FaultCode,
			}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("copy telemetry: %w", err)
	}
	return nil
}

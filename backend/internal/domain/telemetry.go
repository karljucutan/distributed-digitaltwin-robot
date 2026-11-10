package domain

import (
	"errors"
	"fmt"
	"time"
)

type RobotStatus string

const (
	RobotStatusIdle    RobotStatus = "idle"
	RobotStatusMoving  RobotStatus = "moving"
	RobotStatusPaused  RobotStatus = "paused"
	RobotStatusFaulted RobotStatus = "faulted"
	RobotStatusStopped RobotStatus = "stopped"
)

var FaultCodes = [...]string{
	"collision_detected",
	"encoder_error",
}

var ErrInvalidTelemetry = errors.New("invalid telemetry")

type RobotTelemetry struct {
	RobotID                int64
	Timestamp              time.Time
	Sequence               int64
	JointPositionRad       [6]float64
	JointVelocityRadPerSec [6]float64
	Status                 RobotStatus
	FaultCode              string
}

func (t *RobotTelemetry) Validate() error {
	if t.RobotID <= 0 || t.Sequence <= 0 || t.Timestamp.IsZero() {
		return fmt.Errorf("%w: robot=%d sequence=%d timestamp=%s",
			ErrInvalidTelemetry, t.RobotID, t.Sequence, t.Timestamp)
	}

	switch t.Status {
	case RobotStatusIdle, RobotStatusMoving, RobotStatusPaused,
		RobotStatusFaulted, RobotStatusStopped:
		return nil
	default:
		return fmt.Errorf("%w: robot=%d status=%q", ErrInvalidTelemetry, t.RobotID, t.Status)
	}
}

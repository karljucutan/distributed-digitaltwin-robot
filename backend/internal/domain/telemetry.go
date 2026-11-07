package domain

import "time"

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

type RobotTelemetry struct {
	RobotID                int64
	Timestamp              time.Time
	Sequence               int64
	JointPositionRad       [6]float64
	JointVelocityRadPerSec [6]float64
	Status                 RobotStatus
	FaultCode              string
}

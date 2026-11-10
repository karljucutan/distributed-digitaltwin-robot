package domain

import "time"

// RobotTwin is the current state of one robot's digital twin.
type RobotTwin struct {
	RobotID     int64          `json:"robot_id"`
	Version     uint64         `json:"version"`
	Telemetry   RobotTelemetry `json:"telemetry"`
	StatusSince time.Time      `json:"status_since"`
}

// Apply updates the twin from telemetry. It returns false if the telemetry
// is a duplicate or older than the current state.
func (t *RobotTwin) Apply(telemetry RobotTelemetry, version uint64) bool {
	if version <= t.Version {
		return false
	}

	if t.Version == 0 || t.Telemetry.Status != telemetry.Status {
		t.StatusSince = telemetry.Timestamp
	}

	// Add more twin logic here: derived metrics, anomaly detection, rules.

	t.RobotID = telemetry.RobotID
	t.Version = version
	t.Telemetry = telemetry
	return true
}

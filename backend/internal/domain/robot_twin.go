package domain

import "time"

// RobotTwin is the current state of one robot's digital twin.
type RobotTwin struct {
	RobotID            int64          `json:"robot_id"`
	Version            uint64         `json:"version"`
	LastStreamSequence uint64         `json:"last_stream_seq"`
	LastDeviceSequence int64          `json:"last_device_seq"`
	Telemetry          RobotTelemetry `json:"telemetry"`
	StatusSince        time.Time      `json:"status_since"`
}

// Apply updates the twin from telemetry. Observation timestamps order updates
// across device-sequence resets, so producers must use synchronized clocks.
// Device sequence breaks ties; streamSequence is retained for diagnostics.
func (t *RobotTwin) Apply(telemetry RobotTelemetry, streamSequence uint64) bool {
	if !t.Telemetry.Timestamp.IsZero() {
		if telemetry.Timestamp.Before(t.Telemetry.Timestamp) {
			return false
		}
		if telemetry.Timestamp.Equal(t.Telemetry.Timestamp) &&
			telemetry.Sequence <= t.LastDeviceSequence {
			return false
		}
	}

	if t.Telemetry.Timestamp.IsZero() || t.Telemetry.Status != telemetry.Status {
		t.StatusSince = telemetry.Timestamp
	}

	// Add more twin logic here: derived metrics, anomaly detection, rules.

	t.RobotID = telemetry.RobotID
	t.Version++
	t.LastStreamSequence = streamSequence
	t.LastDeviceSequence = telemetry.Sequence
	t.Telemetry = telemetry
	return true
}

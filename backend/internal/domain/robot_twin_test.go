package domain

import (
	"testing"
	"time"
)

func TestRobotTwinApplyInitializesState(t *testing.T) {
	timestamp := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	telemetry := RobotTelemetry{
		RobotID: 42, Timestamp: timestamp, Sequence: 1, Status: RobotStatusMoving,
	}

	var twin RobotTwin
	if !twin.Apply(telemetry, 10) {
		t.Fatal("first telemetry was not applied")
	}
	if twin.RobotID != telemetry.RobotID {
		t.Errorf("RobotID = %d, want %d", twin.RobotID, telemetry.RobotID)
	}
	if twin.Version != 10 {
		t.Errorf("Version = %d, want 10", twin.Version)
	}
	if twin.Telemetry != telemetry {
		t.Errorf("Telemetry = %+v, want %+v", twin.Telemetry, telemetry)
	}
	if !twin.StatusSince.Equal(timestamp) {
		t.Errorf("StatusSince = %s, want %s", twin.StatusSince, timestamp)
	}
}

func TestRobotTwinApplyRejectsNonIncreasingVersion(t *testing.T) {
	timestamp := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	telemetry := RobotTelemetry{
		RobotID: 42, Timestamp: timestamp, Sequence: 1, Status: RobotStatusMoving,
	}

	t.Run("duplicate version", func(t *testing.T) {
		var twin RobotTwin
		twin.Apply(telemetry, 10)

		if twin.Apply(telemetry, 10) {
			t.Fatal("duplicate version was applied")
		}
	})

	t.Run("older version", func(t *testing.T) {
		var twin RobotTwin
		twin.Apply(telemetry, 10)

		if twin.Apply(telemetry, 9) {
			t.Fatal("older version was applied")
		}
	})
}

func TestRobotTwinApplyPreservesStatusSinceWhenStatusIsUnchanged(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	initial := RobotTelemetry{RobotID: 42, Timestamp: start, Sequence: 1, Status: RobotStatusMoving}
	stillMoving := initial
	stillMoving.Timestamp = start.Add(time.Second)
	stillMoving.Sequence++

	var twin RobotTwin
	twin.Apply(initial, 10)
	if !twin.Apply(stillMoving, 11) {
		t.Fatal("newer telemetry was not applied")
	}
	if !twin.StatusSince.Equal(start) {
		t.Errorf("StatusSince = %s, want %s", twin.StatusSince, start)
	}
}

func TestRobotTwinApplyResetsStatusSinceWhenStatusChanges(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	initial := RobotTelemetry{RobotID: 42, Timestamp: start, Sequence: 1, Status: RobotStatusMoving}
	paused := initial
	paused.Timestamp = start.Add(2 * time.Second)
	paused.Sequence++
	paused.Status = RobotStatusPaused

	var twin RobotTwin
	twin.Apply(initial, 10)
	if !twin.Apply(paused, 11) {
		t.Fatal("newer telemetry with changed status was not applied")
	}
	if !twin.StatusSince.Equal(paused.Timestamp) {
		t.Errorf("StatusSince = %s, want %s", twin.StatusSince, paused.Timestamp)
	}
}

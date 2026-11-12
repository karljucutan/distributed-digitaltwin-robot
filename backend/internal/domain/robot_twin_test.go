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
	if twin.Version != 1 {
		t.Errorf("Version = %d, want 1", twin.Version)
	}
	if twin.LastStreamSequence != 10 {
		t.Errorf("LastStreamSequence = %d, want 10", twin.LastStreamSequence)
	}
	if twin.LastDeviceSequence != telemetry.Sequence {
		t.Errorf("LastDeviceSequence = %d, want %d", twin.LastDeviceSequence, telemetry.Sequence)
	}
	if twin.Telemetry != telemetry {
		t.Errorf("Telemetry = %+v, want %+v", twin.Telemetry, telemetry)
	}
	if !twin.StatusSince.Equal(timestamp) {
		t.Errorf("StatusSince = %s, want %s", twin.StatusSince, timestamp)
	}
}

func TestRobotTwinApplyRejectsDuplicateObservation(t *testing.T) {
	timestamp := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	telemetry := RobotTelemetry{
		RobotID: 42, Timestamp: timestamp, Sequence: 1, Status: RobotStatusMoving,
	}

	var twin RobotTwin
	twin.Apply(telemetry, 10)

	// A redelivery can have the same timestamp and device sequence even if
	// processing metadata differs.
	if twin.Apply(telemetry, 11) {
		t.Fatal("duplicate observation was applied")
	}
	if twin.Version != 1 {
		t.Fatalf("Version = %d after duplicate telemetry, want 1", twin.Version)
	}
}

func TestRobotTwinApplyRejectsOlderTimestampEvenWithHigherSequences(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 1, 0, time.UTC)
	initial := RobotTelemetry{
		RobotID: 42, Timestamp: start, Sequence: 10, Status: RobotStatusMoving,
	}
	older := initial
	older.Timestamp = start.Add(-time.Second)
	older.Sequence = 11

	var twin RobotTwin
	twin.Apply(initial, 10)

	if twin.Apply(older, 11) {
		t.Fatal("older observation was applied")
	}
	if twin.Version != 1 {
		t.Fatalf("Version = %d after older telemetry, want 1", twin.Version)
	}
}

func TestRobotTwinApplyAcceptsNewerTimestampAfterDeviceSequenceReset(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	initial := RobotTelemetry{
		RobotID: 42, Timestamp: start, Sequence: 900, Status: RobotStatusMoving,
	}
	afterRestart := RobotTelemetry{
		RobotID: 42, Timestamp: start.Add(time.Second), Sequence: 1, Status: RobotStatusMoving,
	}

	var twin RobotTwin
	twin.Apply(initial, 100)

	if !twin.Apply(afterRestart, 1) {
		t.Fatal("newer observation after device sequence reset was not applied")
	}
	if twin.Version != 2 {
		t.Fatalf("Version = %d, want 2", twin.Version)
	}
	if twin.LastDeviceSequence != 1 {
		t.Errorf("LastDeviceSequence = %d, want 1", twin.LastDeviceSequence)
	}
	if twin.LastStreamSequence != 1 {
		t.Errorf("LastStreamSequence = %d, want 1", twin.LastStreamSequence)
	}
}

func TestRobotTwinApplyUsesDeviceSequenceForEqualTimestamps(t *testing.T) {
	timestamp := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	initial := RobotTelemetry{
		RobotID: 42, Timestamp: timestamp, Sequence: 1, Status: RobotStatusMoving,
	}
	next := initial
	next.Sequence = 2

	var twin RobotTwin
	twin.Apply(initial, 10)

	if !twin.Apply(next, 9) {
		t.Fatal("higher device sequence with equal timestamp was not applied")
	}
	if twin.LastDeviceSequence != 2 {
		t.Errorf("LastDeviceSequence = %d, want 2", twin.LastDeviceSequence)
	}
}

func TestRobotTwinApplyPreservesStatusSinceWhenStatusIsUnchanged(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	initial := RobotTelemetry{RobotID: 42, Timestamp: start, Sequence: 1, Status: RobotStatusMoving}
	stillMoving := initial
	stillMoving.Timestamp = start.Add(time.Second)
	stillMoving.Sequence++

	var twin RobotTwin
	twin.Apply(initial, 10)
	if !twin.Apply(stillMoving, 20) {
		t.Fatal("newer telemetry was not applied")
	}
	if twin.Version != 2 {
		t.Fatalf("Version = %d, want 2", twin.Version)
	}
	if twin.LastStreamSequence != 20 {
		t.Fatalf("LastStreamSequence = %d, want 20", twin.LastStreamSequence)
	}
	if twin.LastDeviceSequence != stillMoving.Sequence {
		t.Fatalf("LastDeviceSequence = %d, want %d", twin.LastDeviceSequence, stillMoving.Sequence)
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
	if twin.Version != 2 {
		t.Fatalf("Version = %d, want 2", twin.Version)
	}
	if !twin.StatusSince.Equal(paused.Timestamp) {
		t.Errorf("StatusSince = %s, want %s", twin.StatusSince, paused.Timestamp)
	}
}

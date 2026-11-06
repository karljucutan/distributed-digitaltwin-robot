package fleetsimulator

import (
	"context"
	"log/slog"
	"math"
	"strconv"
	"sync/atomic"
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

type RobotTwin struct {
	ID          int64
	Name        string
	Sequence    int64
	PhaseOffset float64
}

type RobotTelemetry struct {
	RobotID                int64
	Timestamp              time.Time
	Sequence               int64
	JointPositionRad       [6]float64
	JointVelocityRadPerSec [6]float64
	Status                 RobotStatus
}

type SimulationManager struct {
	AppContext   context.Context
	RobotCounter atomic.Int64
}

func (m *SimulationManager) RunSimulation(robotCount int, ttl time.Duration, tickInterval time.Duration) {
	robots := make([]RobotTwin, robotCount)
	for index := range robotCount {
		id := m.RobotCounter.Add(1)

		robots[index] = RobotTwin{
			ID:   id,
			Name: "Robot_" + strconv.FormatInt(id, 10),
		}
	}

	startedAt := time.Now()
	simulationCtx, cancel := context.WithTimeout(m.AppContext, ttl)
	defer cancel()

	const cycleSeconds = 4.0
	omega := 2 * math.Pi / cycleSeconds
	amplitudes := [6]float64{0.5, 0.4, 0.6, 0.3, 0.25, 0.4}

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-simulationCtx.Done():
			return
		case timestamp := <-ticker.C:
			elapsed := timestamp.Sub(startedAt).Seconds()

			for index := range robots {
				robot := robots[index]
				robot.Sequence++

				telemetry := RobotTelemetry{
					RobotID:   robot.ID,
					Timestamp: timestamp,
					Sequence:  robot.Sequence,
					Status:    RobotStatusMoving,
				}

				for joint, amplitude := range amplitudes {
					phase := omega*elapsed + robot.PhaseOffset + float64(joint)*0.35
					telemetry.JointPositionRad[joint] = amplitude * math.Sin(phase)
					telemetry.JointVelocityRadPerSec[joint] = amplitude * omega * math.Cos(phase)
				}

				// publish to NATS
				slog.Info("publishing to NATS")
			}
		}
	}
}

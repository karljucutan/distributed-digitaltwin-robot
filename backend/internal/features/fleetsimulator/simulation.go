package fleetsimulator

import (
	"context"
	"encoding/json/v2"
	"log/slog"
	"math"
	"math/rand/v2"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/karljucutan/distributed-digitaltwin-robot/internal/domain"
	"github.com/nats-io/nats.go/jetstream"
)

type Robot struct {
	ID            int64
	Name          string
	Sequence      int64
	PhaseOffset   float64
	Status        domain.RobotStatus
	StatusUntil   time.Time
	FaultCode     string
	MotionSeconds float64
}

type SimulationManager struct {
	JetStream    jetstream.JetStream
	RobotCounter atomic.Int64
}

const (
	pauseRatePerSec = 0.02  // About one pause per 50 moving seconds.
	faultRatePerSec = 0.005 // About one fault per 200 moving seconds.
)

const RobotTelemetrySubject = "telemetry.robot.state"

func (m *SimulationManager) RunSimulation(ctx context.Context, robotCount int, ttl time.Duration, tickInterval time.Duration) {
	robots := make([]Robot, robotCount)
	for index := range robotCount {
		id := m.RobotCounter.Add(1)

		robots[index] = Robot{
			ID:          id,
			Name:        "Robot_" + strconv.FormatInt(id, 10),
			Status:      domain.RobotStatusMoving,
			PhaseOffset: rand.Float64() * 2 * math.Pi,
		}
	}

	startedAt := time.Now()
	simulationCtx, cancel := context.WithTimeout(ctx, ttl)
	defer cancel()

	const cycleSeconds = 4.0
	omega := 2 * math.Pi / cycleSeconds
	amplitudes := [6]float64{0.5, 0.4, 0.6, 0.3, 0.25, 0.4}

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	previousTick := startedAt
	for {
		select {
		case <-simulationCtx.Done():
			slog.Info("done")
			return
		case timestamp := <-ticker.C:
			deltaTime := timestamp.Sub(previousTick).Seconds()
			previousTick = timestamp

			for index := range robots {
				robot := &robots[index]
				robot.step(timestamp, deltaTime)
				robot.Sequence++

				telemetry := domain.RobotTelemetry{
					RobotID:   robot.ID,
					Timestamp: timestamp,
					Sequence:  robot.Sequence,
					Status:    robot.Status,
					FaultCode: robot.FaultCode,
				}

				for joint, amplitude := range amplitudes {
					phase := omega*robot.MotionSeconds + robot.PhaseOffset + float64(joint)*0.35
					telemetry.JointPositionRad[joint] = amplitude * math.Sin(phase)

					if robot.Status == domain.RobotStatusMoving {
						telemetry.JointVelocityRadPerSec[joint] =
							amplitude * omega * math.Cos(phase)
					}
				}

				payload, err := json.Marshal(telemetry)
				if err != nil {
					continue
				}

				subject := RobotTelemetrySubject
				if _, err := m.JetStream.PublishAsync(subject, payload); err != nil {
					slog.Error("publish failed", "robot", robot.ID, "err", err)
					continue
				}
			}
		}
	}
}

func (r *Robot) step(now time.Time, deltaTime float64) {
	switch r.Status {
	case domain.RobotStatusMoving:
		r.MotionSeconds += deltaTime

		totalRate := pauseRatePerSec + faultRatePerSec
		eventChance := -math.Expm1(-totalRate * deltaTime)

		if rand.Float64() >= eventChance {
			return
		}

		if rand.Float64() < faultRatePerSec/totalRate {
			r.Status = domain.RobotStatusFaulted
			r.FaultCode = domain.FaultCodes[rand.N(len(domain.FaultCodes))]
			r.StatusUntil = now.Add(3*time.Second + rand.N(2*time.Second))
		} else {
			r.Status = domain.RobotStatusPaused
			r.StatusUntil = now.Add(time.Second + rand.N(2*time.Second))
		}

	case domain.RobotStatusPaused, domain.RobotStatusFaulted:
		if !now.Before(r.StatusUntil) {
			r.Status = domain.RobotStatusMoving
			r.FaultCode = ""
			r.StatusUntil = time.Time{}
		}
	}
}

package fleetsimulator

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"time"
)

type SimulationRequest struct {
	RobotCount int `json:"robot_count"`
	TTLSeconds int `json:"ttl_seconds"`
}

func SimulationHttpHandler(
	ctx context.Context,
	manager *SimulationManager,
	w http.ResponseWriter,
	r *http.Request) {

	var req SimulationRequest
	// v1 version
	// if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
	// 	http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
	// 	return
	// }
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	go manager.RunSimulation(
		ctx,
		req.RobotCount,
		time.Duration(req.TTLSeconds)*time.Second, 10*time.Millisecond)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
}

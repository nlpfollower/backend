package api

import (
	"encoding/json"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	nexusCore "github.com/nlpfollower/deltamind/nexus/core"
	"net/http"
	"time"
)

// SystemStatus represents the overall system state
type SystemStatus string

const (
	SystemStatusIdle      SystemStatus = "idle"
	SystemStatusInference SystemStatus = "inference"
	SystemStatusTraining  SystemStatus = "training"
)

// SessionInfo contains basic session information
type SessionInfo struct {
	SessionID string    `json:"session_id"`
	ModelID   string    `json:"model_id"`
	State     string    `json:"state"`
	StartedAt time.Time `json:"started_at"`
}

// TrainingInfo contains basic training job information
type TrainingInfo struct {
	JobID     string    `json:"job_id"`
	ModelID   string    `json:"model_id"`
	Status    string    `json:"status"`
	Progress  float64   `json:"progress"`
	StartedAt time.Time `json:"started_at"`
}

// GetSystemStatusRequest contains the request for system status
type GetSystemStatusRequest struct {
	AuthToken AuthToken `json:"auth_token"`
}

// GetSystemStatusResponse contains the current system status
type GetSystemStatusResponse struct {
	Status         SystemStatus   `json:"status"`
	ActiveSessions []SessionInfo  `json:"active_sessions,omitempty"`
	ActiveTraining []TrainingInfo `json:"active_training,omitempty"`
}

// GetSystemStatus returns the current infrastructure state for a user
func (router *APIRouter) GetSystemStatus(w http.ResponseWriter, req *http.Request) {
	var statusReq GetSystemStatusRequest
	if err := json.NewDecoder(req.Body).Decode(&statusReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	claims, err := ValidateSessionKey(statusReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	userID, err := db.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	response := GetSystemStatusResponse{
		Status:         SystemStatusIdle,
		ActiveSessions: []SessionInfo{},
		ActiveTraining: []TrainingInfo{},
	}

	// Check for active training jobs - just use the existing training job tracker
	trainingJobsMu.RLock()
	for jobID, info := range trainingJobs {
		if info.UserID == userID {
			// Get model name
			var modelName string
			router.dbManager.View(func(txn *storage.DatabaseTransaction) error {
				model, err := txn.GetModel(info.ModelID)
				if err == nil && model != nil {
					modelName = model.Name
				}
				return nil
			})

			// Get real-time status from Nexus if possible
			status := "running"
			progress := 0.0

			nexusStatusReq := &nexusCore.TrainingStatusRequest{
				JobID: jobID,
			}

			if respChan, err := router.nexusClient.GetTrainingStatus(nexusStatusReq); err == nil {
				select {
				case resp, ok := <-respChan:
					if ok {
						var statusResp nexusCore.TrainingStatusResponse
						if err := json.Unmarshal(resp.Data, &statusResp); err == nil {
							status = statusResp.Status
							progress = statusResp.Progress
						}
					}
				case <-time.After(2 * time.Second):
					// Use defaults if Nexus doesn't respond quickly
				}
			}

			response.ActiveTraining = append(response.ActiveTraining, TrainingInfo{
				JobID:     jobID,
				ModelID:   modelName,
				Status:    status,
				Progress:  progress,
				StartedAt: info.StartedAt,
			})
		}
	}
	trainingJobsMu.RUnlock()

	// Determine overall system status
	if len(response.ActiveTraining) > 0 {
		response.Status = SystemStatusTraining
	}
	// We don't track sessions currently, so we'll skip that part

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

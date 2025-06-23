package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	nexusCore "github.com/nlpfollower/deltamind/nexus/core"
	"github.com/pkg/errors"
)

// CloneModelRequest contains the request data for cloning a model
type CloneModelRequest struct {
	SourceModelID db.Digest `json:"source_model_id"`
	DisplayName   string    `json:"display_name"`
	AuthToken     AuthToken `json:"auth_token"`
}

// CloneModelResponse contains the response after initiating a clone
type CloneModelResponse struct {
	Model      storage.ModelInfo `json:"model"`
	CloneJobID string            `json:"clone_job_id"`
}

// GetCloneStatusRequest contains the request for checking clone status
type GetCloneStatusRequest struct {
	CloneJobID string    `json:"clone_job_id"`
	AuthToken  AuthToken `json:"auth_token"`
}

// GetCloneStatusResponse contains the clone operation status
type GetCloneStatusResponse struct {
	JobID       string     `json:"job_id"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// GetUserModelsRequest for listing user's cloneable models
type GetUserModelsRequest struct {
	IncludeBase bool      `json:"include_base"`
	AuthToken   AuthToken `json:"auth_token"`
}

// GetUserModelsResponse contains the list of models
type GetUserModelsResponse struct {
	Models []storage.ModelInfo `json:"models"`
}

// CloneModel initiates a model cloning operation
func (router *APIRouter) CloneModel(w http.ResponseWriter, req *http.Request) {
	var cloneReq CloneModelRequest
	if err := json.NewDecoder(req.Body).Decode(&cloneReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	claims, err := ValidateSessionKey(cloneReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	userID, err := db.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	var newModel storage.ModelInfo
	cloneJobID := fmt.Sprintf("clone-%d", time.Now().UnixNano())

	err = router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		// Get source model
		sourceModel, err := txn.GetModel(cloneReq.SourceModelID)
		if err != nil {
			return errors.Wrap(err, "failed to get source model")
		}
		if sourceModel == nil {
			return errors.New("source model not found")
		}

		// Generate new model ID
		modelIDBytes, err := GenerateRandomBytes(32)
		if err != nil {
			return errors.Wrap(err, "failed to generate model ID")
		}

		// Determine new model name based on source
		var newModelName string
		var modelType storage.ModelType

		if sourceModel.ModelType == storage.ModelTypeBase {
			// Cloning a base model: llama-70b -> llama-70b-u{userId}-c1
			cloneCount := 1 // TODO: Get actual count from database
			newModelName = fmt.Sprintf("%s-u%s-c%d", sourceModel.BaseModel,
				userID.String()[:8], cloneCount)
			modelType = storage.ModelTypeClone
		} else {
			// Cloning a trained model: reset to a new clone line
			cloneCount := 1 // TODO: Get actual count from database
			newModelName = fmt.Sprintf("%s-u%s-c%d", sourceModel.BaseModel,
				userID.String()[:8], cloneCount)
			modelType = storage.ModelTypeClone
		}

		// Create new model entry
		timeNow := time.Now()
		newModel = storage.ModelInfo{
			ID:           db.NewDigest(modelIDBytes),
			UserID:       userID,
			Name:         newModelName,
			DisplayName:  cloneReq.DisplayName,
			ModelType:    modelType,
			BaseModel:    sourceModel.BaseModel,
			ModelSize:    sourceModel.ModelSize,
			ParentID:     &sourceModel.ID,
			Status:       storage.ModelStatusCloning,
			PhysicalPath: fmt.Sprintf("/mnt/cold-storage/contents/dcp/%s", newModelName),
			CloneJobID:   &cloneJobID,
			CreatedAt:    timeNow,
			UpdatedAt:    timeNow,
		}

		if err := txn.SetModel(userID, &newModel); err != nil {
			return errors.Wrap(err, "failed to create model")
		}

		return nil
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send clone request to Nexus
	cloneRequest := &nexusCore.CloneModelRequest{
		JobID:      cloneJobID,
		SourcePath: "", // Will be filled by nexus from model info
		TargetPath: newModel.PhysicalPath,
		SourceID:   cloneReq.SourceModelID.String(),
		TargetID:   newModel.ID.String(),
	}

	respChan, err := router.nexusClient.EnqueueModelClone(cloneRequest)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start clone operation: %v", err), http.StatusInternalServerError)
		return
	}

	// Start goroutine to handle async response
	go router.handleCloneResponse(newModel.ID, respChan)

	response := CloneModelResponse{
		Model:      newModel,
		CloneJobID: cloneJobID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetCloneStatus checks the status of a clone operation
func (router *APIRouter) GetCloneStatus(w http.ResponseWriter, req *http.Request) {
	var statusReq GetCloneStatusRequest
	if err := json.NewDecoder(req.Body).Decode(&statusReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, err := ValidateSessionKey(statusReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	// Get clone job status from Nexus
	statusRequest := &nexusCore.CloneStatusRequest{
		JobID: statusReq.CloneJobID,
	}

	respChan, err := router.nexusClient.EnqueueCloneStatus(statusRequest)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get clone status: %v", err), http.StatusInternalServerError)
		return
	}

	// Wait for response
	resp, ok := <-respChan
	if !ok {
		http.Error(w, "Failed to receive status response", http.StatusInternalServerError)
		return
	}

	var statusResp nexusCore.CloneStatusResponse
	if err := json.Unmarshal(resp.Data, &statusResp); err != nil {
		http.Error(w, "Failed to parse status response", http.StatusInternalServerError)
		return
	}

	response := GetCloneStatusResponse{
		JobID:       statusResp.JobID,
		Status:      statusResp.Status,
		Error:       statusResp.Error,
		StartedAt:   statusResp.StartedAt,
		CompletedAt: statusResp.CompletedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetUserCloneableModels returns models that the user can clone
func (router *APIRouter) GetUserCloneableModels(w http.ResponseWriter, req *http.Request) {
	var getReq GetUserModelsRequest
	if err := json.NewDecoder(req.Body).Decode(&getReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	claims, err := ValidateSessionKey(getReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	userID, err := db.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	var models []storage.ModelInfo

	err = router.dbManager.View(func(txn *storage.DatabaseTransaction) error {
		// Get user's own models
		userModels, err := txn.GetUserModels(userID, 100, uint64(time.Now().UnixNano()))
		if err != nil {
			return errors.Wrap(err, "failed to get user models")
		}

		// Add user models that are cloneable (ready status)
		for _, model := range userModels {
			if model.Status == storage.ModelStatusReady {
				models = append(models, *model)
			}
		}

		// Optionally include base models
		if getReq.IncludeBase {
			systemUserID := db.NewDigest([]byte("system-base-models"))
			baseModels, err := txn.GetUserModels(systemUserID, 100, uint64(time.Now().UnixNano()))
			if err != nil {
				return errors.Wrap(err, "failed to get base models")
			}
			for _, model := range baseModels {
				if model.Status == storage.ModelStatusReady {
					models = append(models, *model)
				}
			}
		}

		return nil
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := GetUserModelsResponse{
		Models: models,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCloneResponse processes the async response from Nexus
func (router *APIRouter) handleCloneResponse(modelID db.Digest, respChan <-chan *nexusCore.WrappedResponse) {
	resp, ok := <-respChan
	if !ok {
		return
	}

	var cloneResp nexusCore.CloneModelResponse
	if err := json.Unmarshal(resp.Data, &cloneResp); err != nil {
		return
	}

	// Update model status based on response
	router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		model, err := txn.GetModel(modelID)
		if err != nil || model == nil {
			return err
		}

		if cloneResp.Status == nexusCore.ResponseStatusSuccess {
			model.Status = storage.ModelStatusReady
		} else {
			model.Status = storage.ModelStatusError
		}
		model.UpdatedAt = time.Now()

		return txn.SetModel(model.UserID, model)
	})
}

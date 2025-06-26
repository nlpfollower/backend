package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	nexusCore "github.com/nlpfollower/deltamind/nexus/core"
	"github.com/pkg/errors"
)

type StartTrainingRequest struct {
	SourceModelID string    `json:"source_model_id"`
	DatasetPath   string    `json:"dataset_path"`
	LearningRate  float64   `json:"learning_rate"`
	BatchSize     int       `json:"batch_size"`
	NumEpochs     int       `json:"num_epochs"`
	AuthToken     AuthToken `json:"auth_token"`
}

type StartTrainingResponse struct {
	JobID   string `json:"job_id"`
	ModelID string `json:"model_id"`
	Status  string `json:"status"`
}

type GetTrainingStatusRequest struct {
	JobID     string    `json:"job_id"`
	AuthToken AuthToken `json:"auth_token"`
}

type GetTrainingStatusResponse struct {
	JobID       string     `json:"job_id"`
	Status      string     `json:"status"`
	Progress    float64    `json:"progress"`
	Error       string     `json:"error,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func (router *APIRouter) StartTraining(w http.ResponseWriter, req *http.Request) {
	var trainReq StartTrainingRequest
	if err := json.NewDecoder(req.Body).Decode(&trainReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	claims, err := ValidateSessionKey(trainReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	userID, err := db.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	var sourceModel *storage.ModelInfo
	var newModel storage.ModelInfo
	var checkpointPath string

	err = router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		// Get source model by name
		models, err := txn.GetUserModels(userID, 100, uint64(time.Now().UnixNano()))
		if err != nil {
			return errors.Wrap(err, "failed to get user models")
		}

		// Also check base models
		systemUserID := db.NewDigest([]byte("system-base-models"))
		baseModels, err := txn.GetUserModels(systemUserID, 10, uint64(time.Now().UnixNano()))
		if err == nil {
			models = append(models, baseModels...)
		}

		// Find source model by name
		for _, m := range models {
			if m.Name == trainReq.SourceModelID {
				sourceModel = m
				break
			}
		}

		if sourceModel == nil {
			return fmt.Errorf("source model not found: %s", trainReq.SourceModelID)
		}

		// Check permissions
		if sourceModel.UserID != userID && sourceModel.ModelType != storage.ModelTypeBase {
			return errors.New("unauthorized to train from this model")
		}

		// Resolve checkpoint path
		checkpointPath = sourceModel.CheckpointPath
		if checkpointPath == "" {
			return fmt.Errorf("source model has no checkpoint path")
		}

		// Calculate training number
		trainNum := 1
		children, err := txn.GetModelsByParent(sourceModel.ID, 100, uint64(time.Now().UnixNano()))
		if err == nil {
			for _, child := range children {
				if child.ModelType == storage.ModelTypeTrained {
					// Extract training number from name
					parts := strings.Split(child.Name, "-t")
					if len(parts) > 1 {
						var num int
						fmt.Sscanf(parts[len(parts)-1], "%d", &num)
						if num >= trainNum {
							trainNum = num + 1
						}
					}
				}
			}
		}

		// Generate new model name
		baseName := sourceModel.Name
		if sourceModel.ModelType == storage.ModelTypeBase {
			// For base models, create a user-specific trained model
			baseName = fmt.Sprintf("%s-u%s", sourceModel.BaseModel, userID.String()[:8])
		}
		newModelName := fmt.Sprintf("%s-t%d", baseName, trainNum)

		// Generate model ID
		modelIDBytes, err := GenerateRandomBytes(32)
		if err != nil {
			return errors.Wrap(err, "failed to generate model ID")
		}

		// Create new model entry
		timeNow := time.Now()
		newModel = storage.ModelInfo{
			ID:             db.NewDigest(modelIDBytes),
			UserID:         userID,
			Name:           newModelName,
			DisplayName:    fmt.Sprintf("Training %d from %s", trainNum, sourceModel.DisplayName),
			ModelType:      storage.ModelTypeTrained,
			BaseModel:      sourceModel.BaseModel,
			ModelSize:      sourceModel.ModelSize,
			ParentID:       &sourceModel.ID,
			Status:         storage.ModelStatusTraining,
			CheckpointPath: fmt.Sprintf("/mnt/cold/contents/dcp/%s/checkpoint", newModelName),
			CreatedAt:      timeNow,
			UpdatedAt:      timeNow,
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

	// Generate unique job ID
	jobIDBytes, _ := GenerateRandomBytes(16)
	jobID := fmt.Sprintf("job-%x", jobIDBytes)

	// Send training request to Nexus
	trainRequest := &nexusCore.TrainingRequest{
		JobID:          jobID,
		UserID:         userID,
		SourceModelID:  sourceModel.Name,
		TargetModelID:  newModel.Name,
		CheckpointPath: checkpointPath,
		OutputPath:     newModel.CheckpointPath,
		DatasetPath:    trainReq.DatasetPath,
		LearningRate:   trainReq.LearningRate,
		BatchSize:      trainReq.BatchSize,
		NumEpochs:      trainReq.NumEpochs,
	}

	respChan, err := router.nexusClient.EnqueueTraining(trainRequest)
	if err != nil {
		// Update model status to error
		router.updateModelStatus(newModel.ID, userID, storage.ModelStatusError)
		http.Error(w, fmt.Sprintf("Failed to start training: %v", err), http.StatusInternalServerError)
		return
	}

	// Wait for initial response
	resp, ok := <-respChan
	if !ok {
		router.updateModelStatus(newModel.ID, userID, storage.ModelStatusError)
		http.Error(w, "Failed to receive training response", http.StatusInternalServerError)
		return
	}

	// Parse training response
	var trainResp nexusCore.TrainingResponse
	if err := json.Unmarshal(resp.Data, &trainResp); err != nil {
		router.updateModelStatus(newModel.ID, userID, storage.ModelStatusError)
		http.Error(w, "Failed to parse training response", http.StatusInternalServerError)
		return
	}

	// Store job ID -> model ID mapping for status queries
	// TODO: Add secondary index for this

	response := StartTrainingResponse{
		JobID:   trainResp.JobID,
		ModelID: newModel.Name,
		Status:  trainResp.Status,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (router *APIRouter) GetTrainingStatus(w http.ResponseWriter, req *http.Request) {
	var statusReq GetTrainingStatusRequest
	if err := json.NewDecoder(req.Body).Decode(&statusReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, err := ValidateSessionKey(statusReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	// Send status request to Nexus
	nexusStatusReq := &nexusCore.TrainingStatusRequest{
		JobID: statusReq.JobID,
	}

	respChan, err := router.nexusClient.GetTrainingStatus(nexusStatusReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get training status: %v", err), http.StatusInternalServerError)
		return
	}

	// Wait for response
	resp, ok := <-respChan
	if !ok {
		http.Error(w, "Failed to receive status response", http.StatusInternalServerError)
		return
	}

	// Parse status response
	var statusResp nexusCore.TrainingStatusResponse
	if err := json.Unmarshal(resp.Data, &statusResp); err != nil {
		http.Error(w, "Failed to parse status response", http.StatusInternalServerError)
		return
	}

	response := GetTrainingStatusResponse{
		JobID:       statusResp.JobID,
		Status:      statusResp.Status,
		Progress:    statusResp.Progress,
		Error:       statusResp.Error,
		StartedAt:   statusResp.StartedAt,
		CompletedAt: statusResp.CompletedAt,
	}

	// TODO: Update model status in database based on training status
	// if statusResp.Status == "completed" {
	//     router.updateModelStatus(modelID, userID, storage.ModelStatusReady)
	// } else if statusResp.Status == "error" {
	//     router.updateModelStatus(modelID, userID, storage.ModelStatusError)
	// }

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper function to update model status
func (router *APIRouter) updateModelStatus(modelID db.Digest, userID db.Digest, status storage.ModelStatus) error {
	return router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		model, err := txn.GetModel(modelID)
		if err != nil {
			return err
		}
		if model == nil {
			return errors.New("model not found")
		}

		model.Status = status
		model.UpdatedAt = time.Now()

		return txn.SetModel(userID, model)
	})
}

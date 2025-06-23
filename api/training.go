package api

import (
	"time"

	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
)

type StartTrainingRequest struct {
	ModelID      db.Digest `json:"model_id"`
	DisplayName  string    `json:"display_name"`
	DatasetPath  string    `json:"dataset_path"`
	LearningRate float64   `json:"learning_rate"`
	BatchSize    int       `json:"batch_size"`
	NumEpochs    int       `json:"num_epochs"`
	AuthToken    AuthToken `json:"auth_token"`
}

type StartTrainingResponse struct {
	TrainingJobID string            `json:"training_job_id"`
	NewModelID    db.Digest         `json:"new_model_id"`
	Model         storage.ModelInfo `json:"model"`
}

type GetTrainingStatusRequest struct {
	TrainingJobID string    `json:"training_job_id"`
	AuthToken     AuthToken `json:"auth_token"`
}

type GetTrainingStatusResponse struct {
	JobID       string     `json:"job_id"`
	Status      string     `json:"status"`
	Progress    float64    `json:"progress"`
	Error       string     `json:"error,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

//func (router *APIRouter) StartTraining(w http.ResponseWriter, req *http.Request) {
//	var trainReq StartTrainingRequest
//	if err := json.NewDecoder(req.Body).Decode(&trainReq); err != nil {
//		http.Error(w, err.Error(), http.StatusBadRequest)
//		return
//	}
//
//	claims, err := ValidateSessionKey(trainReq.AuthToken.SessionKey)
//	if err != nil {
//		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
//		return
//	}
//
//	userID, err := db.DigestFromString(claims.UserID)
//	if err != nil {
//		http.Error(w, "Invalid user ID", http.StatusBadRequest)
//		return
//	}
//
//	var newModel storage.ModelInfo
//	var sourceModel *storage.ModelInfo
//	trainingJobID := fmt.Sprintf("train-%d", time.Now().UnixNano())
//
//	err = router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
//		// Get source model
//		sourceModel, err = txn.GetModel(trainReq.ModelID)
//		if err != nil {
//			return errors.Wrap(err, "failed to get model")
//		}
//		if sourceModel == nil {
//			return errors.New("model not found")
//		}
//
//		// Validate permissions and model type
//		if sourceModel.UserID != userID {
//			return errors.New("unauthorized to train this model")
//		}
//		if sourceModel.ModelType == storage.ModelTypeBase {
//			return errors.New("cannot train base models directly")
//		}
//		if sourceModel.Status == storage.ModelStatusTraining {
//			return errors.New("model is already being trained")
//		}
//
//		// Calculate training iteration number
//		trainNum := 1
//		if sourceModel.ModelType == storage.ModelTypeTrained {
//			// Extract train number from name (e.g., "llama-8b-u1-c1-t2" -> 2)
//			parts := strings.Split(sourceModel.Name, "-")
//			for _, part := range parts {
//				if strings.HasPrefix(part, "t") {
//					fmt.Sscanf(part, "t%d", &trainNum)
//					trainNum++ // Increment for new iteration
//					break
//				}
//			}
//		}
//
//		// Generate new model entry for the training result
//		modelIDBytes, err := GenerateRandomBytes(32)
//		if err != nil {
//			return errors.Wrap(err, "failed to generate model ID")
//		}
//
//		// Build new model name
//		baseName := sourceModel.Name
//		if sourceModel.ModelType == storage.ModelTypeTrained {
//			// Remove old training suffix
//			if idx := strings.LastIndex(baseName, "-t"); idx != -1 {
//				baseName = baseName[:idx]
//			}
//		}
//		newModelName := fmt.Sprintf("%s-t%d", baseName, trainNum)
//
//		// Create new model entry
//		timeNow := time.Now()
//		newModel = storage.ModelInfo{
//			ID:             db.NewDigest(modelIDBytes),
//			UserID:         userID,
//			Name:           newModelName,
//			DisplayName:    trainReq.DisplayName,
//			ModelType:      storage.ModelTypeTrained,
//			BaseModel:      sourceModel.BaseModel,
//			ModelSize:      sourceModel.ModelSize,
//			ParentID:       &sourceModel.ID,
//			Status:         storage.ModelStatusTraining,
//			CheckpointPath: fmt.Sprintf("/mnt/cold-storage/contents/dcp/%s", newModelName),
//			CreatedAt:      timeNow,
//			UpdatedAt:      timeNow,
//		}
//
//		if err := txn.SetModel(userID, &newModel); err != nil {
//			return errors.Wrap(err, "failed to create model")
//		}
//
//		return nil
//	})
//
//	if err != nil {
//		http.Error(w, err.Error(), http.StatusInternalServerError)
//		return
//	}
//
//	// Send training request to Nexus
//	nexusReq := &core.TrainingRequest{
//		JobID:          trainingJobID,
//		UserID:         userID,
//		SourceModelID:  sourceModel.Name,
//		TargetModelID:  newModel.Name,
//		CheckpointPath: sourceModel.CheckpointPath,
//		OutputPath:     newModel.CheckpointPath,
//		DatasetPath:    trainReq.DatasetPath,
//		LearningRate:   trainReq.LearningRate,
//		BatchSize:      trainReq.BatchSize,
//		NumEpochs:      trainReq.NumEpochs,
//	}
//
//	responseChan, err := router.nexusClient.EnqueueTraining(nexusReq)
//	if err != nil {
//		router.updateModelStatus(newModel.ID, userID, storage.ModelStatusError)
//		http.Error(w, "Failed to start training", http.StatusInternalServerError)
//		return
//	}
//
//	// Wait for initial response
//	select {
//	case resp := <-responseChan:
//		var trainingResp core.TrainingResponse
//		if err := json.Unmarshal(resp.Data, &trainingResp); err != nil {
//			http.Error(w, "Invalid response from training service", http.StatusInternalServerError)
//			return
//		}
//
//		response := StartTrainingResponse{
//			TrainingJobID: trainingJobID,
//			NewModelID:    newModel.ID,
//			Model:         newModel,
//		}
//		json.NewEncoder(w).Encode(response)
//
//	case <-time.After(30 * time.Second):
//		http.Error(w, "Training service timeout", http.StatusGatewayTimeout)
//	}
//}
//
//func (router *APIRouter) GetTrainingStatus(w http.ResponseWriter, req *http.Request) {
//	var statusReq GetTrainingStatusRequest
//	if err := json.NewDecoder(req.Body).Decode(&statusReq); err != nil {
//		http.Error(w, err.Error(), http.StatusBadRequest)
//		return
//	}
//
//	claims, err := ValidateSessionKey(statusReq.AuthToken.SessionKey)
//	if err != nil {
//		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
//		return
//	}
//
//	// Forward to Nexus
//	nexusReq := &core.TrainingStatusRequest{
//		JobID: statusReq.TrainingJobID,
//	}
//
//	responseChan, err := router.nexusClient.GetTrainingStatus(nexusReq)
//	if err != nil {
//		http.Error(w, "Failed to get training status", http.StatusInternalServerError)
//		return
//	}
//
//	select {
//	case resp := <-responseChan:
//		var statusResp core.TrainingStatusResponse
//		if err := json.Unmarshal(resp.Data, &statusResp); err != nil {
//			http.Error(w, "Invalid response from training service", http.StatusInternalServerError)
//			return
//		}
//
//		response := GetTrainingStatusResponse{
//			JobID:       statusResp.JobID,
//			Status:      statusResp.Status,
//			Progress:    statusResp.Progress,
//			Error:       statusResp.Error,
//			StartedAt:   statusResp.StartedAt,
//			CompletedAt: statusResp.CompletedAt,
//		}
//
//		json.NewEncoder(w).Encode(response)
//
//	case <-time.After(5 * time.Second):
//		http.Error(w, "Training service timeout", http.StatusGatewayTimeout)
//	}
//}

// Helper to update model status
func (router *APIRouter) updateModelStatus(modelID, userID db.Digest, status storage.ModelStatus) error {
	return router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		model, err := txn.GetModel(modelID)
		if err != nil || model == nil {
			return err
		}
		model.Status = status
		model.UpdatedAt = time.Now()
		return txn.SetModel(userID, model)
	})
}

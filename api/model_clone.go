package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/pkg/errors"
)

type CloneModelRequest struct {
	SourceModelID db.Digest `json:"source_model_id"`
	DisplayName   string    `json:"display_name"`
	AuthToken     AuthToken `json:"auth_token"`
}

type CloneModelResponse struct {
	Model storage.ModelInfo `json:"model"`
}

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

	err = router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		// Get source model
		sourceModel, err := txn.GetModel(cloneReq.SourceModelID)
		if err != nil {
			return errors.Wrap(err, "failed to get source model")
		}
		if sourceModel == nil {
			return errors.New("source model not found")
		}

		// Check permissions
		if sourceModel.ModelType != storage.ModelTypeBase && sourceModel.UserID != userID {
			return errors.New("unauthorized to clone this model")
		}

		// Get user and increment clone number atomically
		_, cloneNum, err := txn.GetUserAndIncrementCloneNum(userID)
		if err != nil {
			return errors.Wrap(err, "failed to get/update user")
		}

		// Generate model ID
		modelIDBytes, err := GenerateRandomBytes(32)
		if err != nil {
			return errors.Wrap(err, "failed to generate model ID")
		}

		// Generate model name
		newModelName := fmt.Sprintf("%s-u%s-c%d",
			sourceModel.BaseModel,
			userID.String()[:8],
			cloneNum)

		// Inherit checkpoint path
		checkpointPath := sourceModel.CheckpointPath
		if checkpointPath == "" {
			return errors.Errorf("source model %s has no checkpoint path", sourceModel.Name)
		}

		// Create new model
		timeNow := time.Now()
		newModel = storage.ModelInfo{
			ID:             db.NewDigest(modelIDBytes),
			UserID:         userID,
			Name:           newModelName,
			DisplayName:    cloneReq.DisplayName,
			ModelType:      storage.ModelTypeClone,
			BaseModel:      sourceModel.BaseModel,
			ModelSize:      sourceModel.ModelSize,
			ParentID:       &sourceModel.ID,
			Status:         storage.ModelStatusReady,
			CheckpointPath: checkpointPath,
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

	response := CloneModelResponse{Model: newModel}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ResolveCheckpointPath is a simple helper - moved here since it's only used in API layer
func ResolveCheckpointPath(model *storage.ModelInfo) (string, error) {
	if model.CheckpointPath == "" {
		return "", errors.Errorf("model %s has no checkpoint path", model.Name)
	}
	return model.CheckpointPath, nil
}

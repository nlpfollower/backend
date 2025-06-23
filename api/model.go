package api

import (
	"encoding/json"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/pkg/errors"
	"net/http"
	"time"
)

type CreateModelRequest struct {
	Name      string    `json:"name"`
	AuthToken AuthToken `json:"auth_token"`
}

type CreateModelResponse struct {
	Model storage.ModelInfo `json:"model"`
}

func (router *APIRouter) CreateModel(w http.ResponseWriter, req *http.Request) {
	var createReq CreateModelRequest
	if err := json.NewDecoder(req.Body).Decode(&createReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	claims, err := ValidateSessionKey(createReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	userID, err := db.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	modelIDBytes, err := GenerateRandomBytes(32)
	if err != nil {
		http.Error(w, "Failed to generate model ID", http.StatusInternalServerError)
		return
	}

	var model storage.ModelInfo
	err = router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		timeNow := time.Now()
		model = storage.ModelInfo{
			ID:        db.NewDigest(modelIDBytes),
			UserID:    userID,
			Name:      createReq.Name,
			CreatedAt: timeNow,
			UpdatedAt: timeNow,
		}

		if err := txn.SetModel(userID, &model); err != nil {
			return errors.Wrap(err, "Failed to create model")
		}

		return nil
	})

	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to create model").Error(), http.StatusInternalServerError)
		return
	}

	response := CreateModelResponse{Model: model}
	json.NewEncoder(w).Encode(response)
}

type GetModelsRequest struct {
	Limit        int       `json:"limit"`
	MaxTimestamp uint64    `json:"max_timestamp"`
	IncludeBase  bool      `json:"include_base"` // Add this field
	AuthToken    AuthToken `json:"auth_token"`
}

type GetModelsResponse struct {
	Models           []*storage.ModelInfo `json:"models"`
	NextMaxTimestamp *uint64              `json:"next_max_timestamp,omitempty"`
}

// Update the GetModels function to include base models when requested:
func (router *APIRouter) GetModels(w http.ResponseWriter, req *http.Request) {
	var getReq GetModelsRequest
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

	var response GetModelsResponse
	err = router.dbManager.View(func(txn *storage.DatabaseTransaction) error {
		if getReq.Limit == 0 {
			getReq.Limit = 10
		}

		if getReq.MaxTimestamp == 0 {
			getReq.MaxTimestamp = uint64(time.Now().UnixNano())
		}

		models, err := txn.GetUserModels(userID, getReq.Limit, getReq.MaxTimestamp)
		if err != nil {
			return errors.Wrap(err, "Failed to get models")
		}

		response.Models = models

		// Include base models if requested
		if getReq.IncludeBase {
			systemUserID := db.NewDigest([]byte("system-base-models"))
			baseModels, err := txn.GetUserModels(systemUserID, 10, uint64(time.Now().UnixNano()))
			if err != nil {
				return errors.Wrap(err, "Failed to get base models")
			}
			response.Models = append(response.Models, baseModels...)
		}

		if len(models) > 0 {
			lastModelTime := uint64(models[len(models)-1].UpdatedAt.UnixNano())
			response.NextMaxTimestamp = &lastModelTime
		}

		return nil
	})

	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get models").Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(response)
}

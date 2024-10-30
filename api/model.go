package api

import (
	"encoding/json"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/pkg/errors"
	"math"
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
	AuthToken    AuthToken `json:"auth_token"`
}

type GetModelsResponse struct {
	Models           []*storage.ModelInfo `json:"models"`
	NextMaxTimestamp *uint64              `json:"next_max_timestamp,omitempty"`
}

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

type DeleteModelRequest struct {
	ModelID   db.Digest `json:"model_id"`
	AuthToken AuthToken `json:"auth_token"`
}

type DeleteModelResponse struct {
	Success bool `json:"success"`
}

func (router *APIRouter) DeleteModel(w http.ResponseWriter, req *http.Request) {
	var deleteReq DeleteModelRequest
	if err := json.NewDecoder(req.Body).Decode(&deleteReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	claims, err := ValidateSessionKey(deleteReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	userID, err := db.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	err = router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		model, err := txn.GetModel(deleteReq.ModelID)
		if err != nil {
			return errors.Wrap(err, "Failed to get model")
		}
		if model == nil {
			return errors.New("Model not found")
		}
		if model.UserID != userID {
			return errors.New("User does not have permission to delete this model")
		}

		if err := txn.DeleteModel(deleteReq.ModelID); err != nil {
			return errors.Wrap(err, "Failed to delete model")
		}

		return nil
	})

	if err != nil {
		switch {
		case err.Error() == "Model not found":
			http.Error(w, err.Error(), http.StatusNotFound)
		case err.Error() == "User does not have permission to delete this model":
			http.Error(w, err.Error(), http.StatusForbidden)
		default:
			http.Error(w, errors.Wrap(err, "Failed to delete model").Error(), http.StatusInternalServerError)
		}
		return
	}

	response := DeleteModelResponse{Success: true}
	json.NewEncoder(w).Encode(response)
}

type CreateModelIterationRequest struct {
	ModelID     db.Digest `json:"model_id"`
	Description string    `json:"description"`
	AuthToken   AuthToken `json:"auth_token"`
}

type CreateModelIterationResponse struct {
	Iteration storage.ModelIteration `json:"iteration"`
}

func (router *APIRouter) CreateModelIteration(w http.ResponseWriter, req *http.Request) {
	var createReq CreateModelIterationRequest
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

	iterationIDBytes, err := GenerateRandomBytes(32)
	if err != nil {
		http.Error(w, "Failed to generate iteration ID", http.StatusInternalServerError)
		return
	}

	var iteration storage.ModelIteration
	err = router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		model, err := txn.GetModel(createReq.ModelID)
		if err != nil {
			return errors.Wrap(err, "Failed to get model")
		}
		if model == nil {
			return errors.New("Model not found")
		}
		if model.UserID != userID {
			return errors.New("User does not have permission to create iterations for this model")
		}

		// Get the current iterations to determine the next index
		currentIterations, err := txn.GetModelIterations(createReq.ModelID, math.MaxUint64, 1)
		if err != nil {
			return errors.Wrap(err, "Failed to get current iterations")
		}

		var nextIndex uint64
		if len(currentIterations) > 0 {
			nextIndex = currentIterations[0].Index + 1
		}

		iteration = storage.ModelIteration{
			ID:          db.NewDigest(iterationIDBytes),
			ModelID:     createReq.ModelID,
			Description: createReq.Description,
			CreatedAt:   time.Now(),
			Index:       nextIndex,
		}

		if err := txn.SetModelIteration(createReq.ModelID, &iteration); err != nil {
			return errors.Wrap(err, "Failed to create model iteration")
		}

		return nil
	})

	if err != nil {
		switch {
		case err.Error() == "Model not found":
			http.Error(w, err.Error(), http.StatusNotFound)
		case err.Error() == "User does not have permission to create iterations for this model":
			http.Error(w, err.Error(), http.StatusForbidden)
		default:
			http.Error(w, errors.Wrap(err, "Failed to create model iteration").Error(), http.StatusInternalServerError)
		}
		return
	}

	response := CreateModelIterationResponse{Iteration: iteration}
	json.NewEncoder(w).Encode(response)
}

type GetModelIterationsRequest struct {
	ModelID   db.Digest `json:"model_id"`
	MaxIndex  uint64    `json:"max_index"`
	Limit     uint64    `json:"limit"`
	AuthToken AuthToken `json:"auth_token"`
}

type GetModelIterationsResponse struct {
	Iterations []*storage.ModelIteration `json:"iterations"`
}

func (router *APIRouter) GetModelIterations(w http.ResponseWriter, req *http.Request) {
	var getReq GetModelIterationsRequest
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

	var response GetModelIterationsResponse
	err = router.dbManager.View(func(txn *storage.DatabaseTransaction) error {
		model, err := txn.GetModel(getReq.ModelID)
		if err != nil {
			return errors.Wrap(err, "Failed to get model")
		}
		if model == nil {
			return errors.New("Model not found")
		}
		if model.UserID != userID {
			return errors.New("User does not have permission to view iterations for this model")
		}

		if getReq.MaxIndex == 0 {
			getReq.MaxIndex = math.MaxUint64
		}
		if getReq.Limit == 0 {
			getReq.Limit = 10
		}

		iterations, err := txn.GetModelIterations(getReq.ModelID, getReq.MaxIndex, getReq.Limit)
		if err != nil {
			return errors.Wrap(err, "Failed to get model iterations")
		}

		response.Iterations = iterations
		return nil
	})

	if err != nil {
		switch {
		case err.Error() == "Model not found":
			http.Error(w, err.Error(), http.StatusNotFound)
		case err.Error() == "User does not have permission to view iterations for this model":
			http.Error(w, err.Error(), http.StatusForbidden)
		default:
			http.Error(w, errors.Wrap(err, "Failed to get model iterations").Error(), http.StatusInternalServerError)
		}
		return
	}

	json.NewEncoder(w).Encode(response)
}

package api

import (
	"encoding/json"
	"github.com/nlpfollower/deltamind/backend/storage"
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

	// Validate the AuthToken
	claims, err := ValidateSessionKey(createReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	// Get the user ID from the claims
	userID, err := storage.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Generate a random model ID
	modelIDBytes, err := GenerateRandomBytes(32)
	if err != nil {
		http.Error(w, "Failed to generate model ID", http.StatusInternalServerError)
		return
	}

	timeNow := time.Now()
	model := storage.ModelInfo{
		ID:        storage.NewDigest(modelIDBytes),
		UserID:    userID,
		Name:      createReq.Name,
		CreatedAt: timeNow,
		UpdatedAt: timeNow,
	}

	if err := router.dbManager.CreateModel(userID, &model); err != nil {
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

	// Validate the AuthToken
	claims, err := ValidateSessionKey(getReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	// Get the user ID from the claims
	userID, err := storage.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	if getReq.Limit == 0 {
		getReq.Limit = 10 // Default limit
	}

	if getReq.MaxTimestamp == 0 {
		getReq.MaxTimestamp = uint64(time.Now().UnixNano())
	}

	models, err := router.dbManager.GetUserModels(userID, getReq.Limit, getReq.MaxTimestamp)
	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get models").Error(), http.StatusInternalServerError)
		return
	}

	response := GetModelsResponse{Models: models}

	if len(models) > 0 {
		lastModelTime := uint64(models[len(models)-1].UpdatedAt.UnixNano())
		response.NextMaxTimestamp = &lastModelTime
	}

	json.NewEncoder(w).Encode(response)
}

type DeleteModelRequest struct {
	ModelID   storage.Digest `json:"model_id"`
	AuthToken AuthToken      `json:"auth_token"`
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

	// Validate the AuthToken
	claims, err := ValidateSessionKey(deleteReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	// Get the user ID from the claims
	userID, err := storage.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Get the model to ensure it belongs to the user
	model, err := router.dbManager.GetModel(deleteReq.ModelID)
	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get model").Error(), http.StatusInternalServerError)
		return
	}
	if model == nil {
		http.Error(w, "Model not found", http.StatusNotFound)
		return
	}
	if model.UserID != userID {
		http.Error(w, "User does not have permission to delete this model", http.StatusForbidden)
		return
	}

	if err := router.dbManager.DeleteModel(deleteReq.ModelID); err != nil {
		http.Error(w, errors.Wrap(err, "Failed to delete model").Error(), http.StatusInternalServerError)
		return
	}

	response := DeleteModelResponse{Success: true}
	json.NewEncoder(w).Encode(response)
}

type CreateModelIterationRequest struct {
	ModelID     storage.Digest `json:"model_id"`
	Description string         `json:"description"`
	AuthToken   AuthToken      `json:"auth_token"`
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

	// Validate the AuthToken
	claims, err := ValidateSessionKey(createReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	// Get the user ID from the claims
	userID, err := storage.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Get the model to ensure it belongs to the user
	model, err := router.dbManager.GetModel(createReq.ModelID)
	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get model").Error(), http.StatusInternalServerError)
		return
	}
	if model == nil {
		http.Error(w, "Model not found", http.StatusNotFound)
		return
	}
	if model.UserID != userID {
		http.Error(w, "User does not have permission to create iterations for this model", http.StatusForbidden)
		return
	}

	// Generate a random iteration ID
	iterationIDBytes, err := GenerateRandomBytes(32)
	if err != nil {
		http.Error(w, "Failed to generate iteration ID", http.StatusInternalServerError)
		return
	}

	// Get the current iterations to determine the next index
	currentIterations, err := router.dbManager.GetModelIterations(createReq.ModelID, math.MaxUint64, 1)
	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get current iterations").Error(), http.StatusInternalServerError)
		return
	}

	var nextIndex uint64
	if len(currentIterations) > 0 {
		nextIndex = currentIterations[0].Index + 1
	}

	iteration := storage.ModelIteration{
		ID:          storage.NewDigest(iterationIDBytes),
		ModelID:     createReq.ModelID,
		Description: createReq.Description,
		CreatedAt:   time.Now(),
		Index:       nextIndex,
	}

	if err := router.dbManager.CreateModelIteration(createReq.ModelID, &iteration); err != nil {
		http.Error(w, errors.Wrap(err, "Failed to create model iteration").Error(), http.StatusInternalServerError)
		return
	}

	response := CreateModelIterationResponse{Iteration: iteration}
	json.NewEncoder(w).Encode(response)
}

type GetModelIterationsRequest struct {
	ModelID   storage.Digest `json:"model_id"`
	MaxIndex  uint64         `json:"max_index"`
	Limit     uint64         `json:"limit"`
	AuthToken AuthToken      `json:"auth_token"`
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

	// Validate the AuthToken
	claims, err := ValidateSessionKey(getReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	// Get the user ID from the claims
	userID, err := storage.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Get the model to ensure it belongs to the user
	model, err := router.dbManager.GetModel(getReq.ModelID)
	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get model").Error(), http.StatusInternalServerError)
		return
	}
	if model == nil {
		http.Error(w, "Model not found", http.StatusNotFound)
		return
	}
	if model.UserID != userID {
		http.Error(w, "User does not have permission to view iterations for this model", http.StatusForbidden)
		return
	}

	if getReq.MaxIndex == 0 {
		getReq.MaxIndex = math.MaxUint64
	}
	if getReq.Limit == 0 {
		getReq.Limit = 10 // Default limit
	}

	iterations, err := router.dbManager.GetModelIterations(getReq.ModelID, getReq.MaxIndex, getReq.Limit)
	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get model iterations").Error(), http.StatusInternalServerError)
		return
	}

	response := GetModelIterationsResponse{Iterations: iterations}
	json.NewEncoder(w).Encode(response)
}

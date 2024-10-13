package api

import (
	"encoding/json"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/pkg/errors"
	"net/http"
	"time"
)

type CreateSpaceRequest struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	AuthToken   AuthToken `json:"auth_token"`
}

type CreateSpaceResponse struct {
	Space storage.Space `json:"space"`
}

func (router *APIRouter) CreateSpace(w http.ResponseWriter, req *http.Request) {
	var createReq CreateSpaceRequest
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

	// Generate a random space ID
	spaceIDBytes, err := GenerateRandomBytes(32)
	if err != nil {
		http.Error(w, "Failed to generate space ID", http.StatusInternalServerError)
		return
	}

	timeNow := time.Now()
	space := storage.Space{
		ID:          storage.NewDigest(spaceIDBytes),
		UserID:      userID,
		Name:        createReq.Name,
		Description: createReq.Description,
		CreatedAt:   timeNow,
		UpdatedAt:   timeNow,
	}

	if err := router.dbManager.CreateSpace(userID, &space); err != nil {
		http.Error(w, errors.Wrap(err, "Failed to create space").Error(), http.StatusInternalServerError)
		return
	}

	response := CreateSpaceResponse{Space: space}
	json.NewEncoder(w).Encode(response)
}

type GetSpacesRequest struct {
	Limit        int       `json:"limit"`
	MaxTimestamp uint64    `json:"max_timestamp"`
	AuthToken    AuthToken `json:"auth_token"`
}

type GetSpacesResponse struct {
	Spaces           []*storage.Space `json:"spaces"`
	NextMaxTimestamp *uint64          `json:"next_max_timestamp,omitempty"`
}

func (router *APIRouter) GetSpaces(w http.ResponseWriter, req *http.Request) {
	var getReq GetSpacesRequest
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

	spaces, err := router.dbManager.GetUserSpaces(userID, getReq.Limit, getReq.MaxTimestamp)
	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get spaces").Error(), http.StatusInternalServerError)
		return
	}

	response := GetSpacesResponse{Spaces: spaces}

	if len(spaces) > 0 {
		lastSpaceTime := uint64(spaces[len(spaces)-1].UpdatedAt.UnixNano())
		response.NextMaxTimestamp = &lastSpaceTime
	}
	json.NewEncoder(w).Encode(response)
}

type DeleteSpaceRequest struct {
	SpaceID   storage.Digest `json:"space_id"`
	AuthToken AuthToken      `json:"auth_token"`
}

type DeleteSpaceResponse struct {
	Success bool `json:"success"`
}

func (router *APIRouter) DeleteSpace(w http.ResponseWriter, req *http.Request) {
	var deleteReq DeleteSpaceRequest
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

	// Get the space to ensure it belongs to the user
	space, err := router.dbManager.GetSpace(deleteReq.SpaceID)
	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get space").Error(), http.StatusInternalServerError)
		return
	}
	if space == nil {
		http.Error(w, "Space not found", http.StatusNotFound)
		return
	}
	if space.UserID != userID {
		http.Error(w, "User does not have permission to delete this space", http.StatusForbidden)
		return
	}

	if err := router.dbManager.DeleteSpace(deleteReq.SpaceID); err != nil {
		http.Error(w, errors.Wrap(err, "Failed to delete space").Error(), http.StatusInternalServerError)
		return
	}

	response := DeleteSpaceResponse{Success: true}
	json.NewEncoder(w).Encode(response)
}

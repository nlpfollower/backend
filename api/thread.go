package api

import (
	"encoding/json"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/pkg/errors"
	"net/http"
	"time"
)

type CreateThreadRequest struct {
	SpaceID   storage.Digest `json:"space_id"`
	Title     string         `json:"title"`
	AuthToken AuthToken      `json:"auth_token"`
}

type CreateThreadResponse struct {
	Thread storage.Thread `json:"thread"`
}

func (router *APIRouter) CreateThread(w http.ResponseWriter, req *http.Request) {
	var createReq CreateThreadRequest
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

	// Verify the space exists and belongs to the user
	space, err := router.dbManager.GetSpace(createReq.SpaceID)
	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get space").Error(), http.StatusInternalServerError)
		return
	}
	if space == nil {
		http.Error(w, "Space not found", http.StatusNotFound)
		return
	}
	if space.UserID != userID {
		http.Error(w, "User does not have permission to create threads in this space", http.StatusForbidden)
		return
	}

	// Generate a random thread ID
	threadIDBytes, err := GenerateRandomBytes(32) // 32 bytes for a 256-bit ID
	if err != nil {
		http.Error(w, "Failed to generate thread ID", http.StatusInternalServerError)
		return
	}

	timeNow := time.Now()
	thread := storage.Thread{
		ID:        storage.NewDigest(threadIDBytes),
		SpaceID:   createReq.SpaceID,
		Title:     createReq.Title,
		CreatedAt: timeNow,
		UpdatedAt: timeNow,
	}

	if err := router.dbManager.CreateThread(createReq.SpaceID, &thread); err != nil {
		http.Error(w, errors.Wrap(err, "Failed to create thread").Error(), http.StatusInternalServerError)
		return
	}

	response := CreateThreadResponse{Thread: thread}
	json.NewEncoder(w).Encode(response)
}

type GetThreadsRequest struct {
	SpaceID      storage.Digest `json:"space_id"`
	Limit        int            `json:"limit"`
	MaxTimestamp uint64         `json:"max_timestamp"`
	AuthToken    AuthToken      `json:"auth_token"`
}

type GetThreadsResponse struct {
	Threads          []*storage.Thread `json:"threads"`
	NextMaxTimestamp *uint64           `json:"next_max_timestamp,omitempty"`
}

func (router *APIRouter) GetThreads(w http.ResponseWriter, req *http.Request) {
	var getReq GetThreadsRequest
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

	// Verify the space exists and belongs to the user
	space, err := router.dbManager.GetSpace(getReq.SpaceID)
	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get space").Error(), http.StatusInternalServerError)
		return
	}
	if space == nil {
		http.Error(w, "Space not found", http.StatusNotFound)
		return
	}
	if space.UserID != userID {
		http.Error(w, "User does not have permission to view threads in this space", http.StatusForbidden)
		return
	}

	if getReq.Limit == 0 {
		getReq.Limit = 10 // Default limit
	}

	if getReq.MaxTimestamp == 0 {
		getReq.MaxTimestamp = uint64(time.Now().UnixNano())
	}

	threads, err := router.dbManager.GetSpaceThreads(getReq.SpaceID, getReq.Limit, getReq.MaxTimestamp)
	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get threads").Error(), http.StatusInternalServerError)
		return
	}

	response := GetThreadsResponse{Threads: threads}

	if len(threads) > 0 {
		lastThreadTime := uint64(threads[len(threads)-1].UpdatedAt.UnixNano())
		response.NextMaxTimestamp = &lastThreadTime
	}

	json.NewEncoder(w).Encode(response)
}

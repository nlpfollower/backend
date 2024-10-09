package api

import (
	"encoding/json"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/pkg/errors"
	"net/http"
	"time"
)

type CreateMessageRequest struct {
	ThreadID  storage.Digest             `json:"thread_id"`
	ParentID  *storage.CompoundMessageID `json:"parent_id,omitempty"`
	Author    string                     `json:"author"`
	Content   string                     `json:"content"`
	AuthToken AuthToken                  `json:"auth_token"`
}

type CreateMessageResponse struct {
	Message *storage.CompoundMessage `json:"message"`
}

func (router *APIRouter) CreateMessage(w http.ResponseWriter, req *http.Request) {
	var createReq CreateMessageRequest
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
	// Get the thread
	thread, err := router.dbManager.GetThread(createReq.ThreadID)
	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get thread").Error(), http.StatusInternalServerError)
		return
	}
	// Verify the user in the AuthToken is the same as the user who created the thread
	userID, err := storage.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}
	if thread.UserID != userID {
		http.Error(w, "User does not have permission to create messages in this thread", http.StatusUnauthorized)
		return
	}
	// Verify that ParentID is a valid message in the thread, if it is not nil
	if createReq.ParentID != nil {
		if _, err := router.dbManager.GetMessage(createReq.ParentID.ID); err != nil {
			http.Error(w, errors.Wrap(err, "Failed to get parent message").Error(), http.StatusInternalServerError)
			return
		}
	}

	messageIDBytes, err := GenerateRandomBytes(32) // 32 bytes for a 256-bit ID
	if err != nil {
		http.Error(w, "Failed to generate message ID", http.StatusInternalServerError)
		return
	}

	// Create the CompoundMessage
	timeNow := time.Now()
	message := &storage.CompoundMessage{
		ID:        storage.NewDigest(messageIDBytes), // Generate a unique ID
		ThreadID:  createReq.ThreadID,
		ParentID:  createReq.ParentID,
		Author:    createReq.Author,
		Messages:  []string{createReq.Content},
		CreatedAt: timeNow,
		UpdatedAt: timeNow,
	}

	if err := router.dbManager.CreateMessage(createReq.ThreadID, message); err != nil {
		http.Error(w, errors.Wrap(err, "Failed to create message").Error(), http.StatusInternalServerError)
		return
	}

	response := CreateMessageResponse{Message: message}
	json.NewEncoder(w).Encode(response)
}

type GetMessagesRequest struct {
	ThreadID     storage.Digest `json:"thread_id"`
	Limit        int            `json:"limit"`
	MaxTimestamp uint64         `json:"max_timestamp"`
	AuthToken    AuthToken      `json:"auth_token"`
}

type GetMessagesResponse struct {
	Messages         []*storage.CompoundMessage `json:"messages"`
	NextMaxTimestamp *uint64                    `json:"next_max_timestamp,omitempty"`
}

func (router *APIRouter) GetMessages(w http.ResponseWriter, req *http.Request) {
	var getReq GetMessagesRequest
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
	// Get the thread
	thread, err := router.dbManager.GetThread(getReq.ThreadID)
	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get thread").Error(), http.StatusInternalServerError)
		return
	}
	// Verify the user in the AuthToken is the same as the user who created the thread
	if thread.UserID != userID {
		http.Error(w, "User does not have permission to view messages in this thread", http.StatusUnauthorized)
		return
	}

	if getReq.Limit == 0 {
		getReq.Limit = 50 // Default limit
	}

	if getReq.MaxTimestamp == 0 {
		getReq.MaxTimestamp = uint64(time.Now().UnixNano())
	}

	messages, err := router.dbManager.GetThreadMessages(getReq.ThreadID, getReq.Limit, getReq.MaxTimestamp)
	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get messages").Error(), http.StatusInternalServerError)
		return
	}

	response := GetMessagesResponse{Messages: messages}

	if len(messages) > 0 {
		lastMessageTime := uint64(messages[len(messages)-1].UpdatedAt.UnixNano())
		response.NextMaxTimestamp = &lastMessageTime
	}

	json.NewEncoder(w).Encode(response)
}

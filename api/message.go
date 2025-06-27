package api

import (
	"encoding/json"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/pkg/errors"
	"net/http"
	"time"
)

type CreateMessageRequest struct {
	ThreadID    db.Digest                   `json:"thread_id"`
	ParentID    *storage.CompoundMessageID  `json:"parent_id,omitempty"`
	Author      string                      `json:"author"`
	Content     string                      `json:"content"`
	Attachments []storage.MessageAttachment `json:"attachments,omitempty"`
	AuthToken   AuthToken                   `json:"auth_token"`
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

	messageIDBytes, err := GenerateRandomBytes(32)
	if err != nil {
		http.Error(w, "Failed to generate message ID", http.StatusInternalServerError)
		return
	}

	var message *storage.CompoundMessage
	err = router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		// Get the thread
		thread, err := txn.GetThread(createReq.ThreadID)
		if err != nil {
			return errors.Wrap(err, "Failed to get thread")
		}
		if thread == nil {
			return errors.New("Thread not found")
		}

		// Get the space
		space, err := txn.GetSpace(thread.SpaceID)
		if err != nil {
			return errors.Wrap(err, "Failed to get space")
		}
		if space.UserID != userID {
			return errors.New("User does not have permission to create messages in this space")
		}

		// Verify parent message if provided
		if createReq.ParentID != nil {
			parentMsg, err := txn.GetMessage(createReq.ParentID.ID)
			if err != nil {
				return errors.Wrap(err, "Failed to get parent message")
			}
			if parentMsg == nil {
				return errors.New("Parent message not found")
			}
		}

		timeNow := time.Now()
		message = &storage.CompoundMessage{
			ID:          db.NewDigest(messageIDBytes),
			ThreadID:    createReq.ThreadID,
			ParentID:    createReq.ParentID,
			Author:      createReq.Author,
			Messages:    []string{createReq.Content},
			Attachments: createReq.Attachments,
			CreatedAt:   timeNow,
			UpdatedAt:   timeNow,
		}

		if err := txn.SetMessage(createReq.ThreadID, message); err != nil {
			return errors.Wrap(err, "Failed to create message")
		}

		// Update parent thread timestamp
		if err := router.updateThreadWithTxn(txn, thread.ID, userID, thread.Title, timeNow); err != nil {
			return errors.Wrap(err, "Failed to update parent thread")
		}

		return nil
	})

	if err != nil {
		switch {
		case err.Error() == "Thread not found":
			http.Error(w, err.Error(), http.StatusNotFound)
		case err.Error() == "Parent message not found":
			http.Error(w, err.Error(), http.StatusNotFound)
		case err.Error() == "User does not have permission to create messages in this space":
			http.Error(w, err.Error(), http.StatusForbidden)
		default:
			http.Error(w, errors.Wrap(err, "Failed to create message").Error(), http.StatusInternalServerError)
		}
		return
	}

	response := CreateMessageResponse{Message: message}
	json.NewEncoder(w).Encode(response)
}

type GetMessagesRequest struct {
	ThreadID     db.Digest `json:"thread_id"`
	Limit        int       `json:"limit"`
	MaxTimestamp uint64    `json:"max_timestamp"`
	AuthToken    AuthToken `json:"auth_token"`
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

	var response GetMessagesResponse
	err = router.dbManager.View(func(txn *storage.DatabaseTransaction) error {
		// Get the thread
		thread, err := txn.GetThread(getReq.ThreadID)
		if err != nil {
			return errors.Wrap(err, "Failed to get thread")
		}
		if thread == nil {
			return errors.New("Thread not found")
		}

		// Get the space
		space, err := txn.GetSpace(thread.SpaceID)
		if err != nil {
			return errors.Wrap(err, "Failed to get space")
		}
		if space == nil {
			return errors.New("Space not found")
		}
		if space.UserID != userID {
			return errors.New("User does not have permission to view messages in this space")
		}

		if getReq.Limit == 0 {
			getReq.Limit = 50
		}

		if getReq.MaxTimestamp == 0 {
			getReq.MaxTimestamp = uint64(time.Now().UnixNano())
		}

		messages, err := txn.GetThreadMessages(getReq.ThreadID, getReq.Limit, getReq.MaxTimestamp)
		if err != nil {
			return errors.Wrap(err, "Failed to get messages")
		}

		response.Messages = messages
		if len(messages) > 0 {
			lastMessageTime := uint64(messages[len(messages)-1].UpdatedAt.UnixNano())
			response.NextMaxTimestamp = &lastMessageTime
		}

		return nil
	})

	if err != nil {
		switch {
		case err.Error() == "Thread not found":
			http.Error(w, err.Error(), http.StatusNotFound)
		case err.Error() == "Space not found":
			http.Error(w, err.Error(), http.StatusNotFound)
		case err.Error() == "User does not have permission to view messages in this space":
			http.Error(w, err.Error(), http.StatusForbidden)
		default:
			http.Error(w, errors.Wrap(err, "Failed to get messages").Error(), http.StatusInternalServerError)
		}
		return
	}

	json.NewEncoder(w).Encode(response)
}

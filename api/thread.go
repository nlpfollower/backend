package api

import (
	"encoding/json"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/pkg/errors"
	"net/http"
	"time"
)

type CreateThreadRequest struct {
	SpaceID   db.Digest `json:"space_id"`
	Title     string    `json:"title"`
	AuthToken AuthToken `json:"auth_token"`
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

	threadIDBytes, err := GenerateRandomBytes(32)
	if err != nil {
		http.Error(w, "Failed to generate thread ID", http.StatusInternalServerError)
		return
	}

	var thread storage.Thread
	err = router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		// Verify the space exists and belongs to the user
		space, err := txn.GetSpace(createReq.SpaceID)
		if err != nil {
			return errors.Wrap(err, "Failed to get space")
		}
		if space == nil {
			return errors.New("Space not found")
		}
		if space.UserID != userID {
			return errors.New("User does not have permission to create threads in this space")
		}

		timeNow := time.Now()
		thread = storage.Thread{
			ID:        db.NewDigest(threadIDBytes),
			SpaceID:   createReq.SpaceID,
			Title:     createReq.Title,
			CreatedAt: timeNow,
			UpdatedAt: timeNow,
		}

		if err := txn.SetThread(createReq.SpaceID, &thread); err != nil {
			return errors.Wrap(err, "Failed to create thread")
		}

		// Update parent space timestamp
		if err := router.updateSpaceWithTxn(txn, createReq.SpaceID, userID, space.Name, space.Description, timeNow); err != nil {
			return errors.Wrap(err, "Failed to update parent space")
		}

		return nil
	})

	if err != nil {
		switch {
		case err.Error() == "Space not found":
			http.Error(w, err.Error(), http.StatusNotFound)
		case err.Error() == "User does not have permission to create threads in this space":
			http.Error(w, err.Error(), http.StatusForbidden)
		default:
			http.Error(w, errors.Wrap(err, "Failed to create thread").Error(), http.StatusInternalServerError)
		}
		return
	}

	response := CreateThreadResponse{Thread: thread}
	json.NewEncoder(w).Encode(response)
}

type GetThreadsRequest struct {
	SpaceID      db.Digest `json:"space_id"`
	Limit        int       `json:"limit"`
	MaxTimestamp uint64    `json:"max_timestamp"`
	AuthToken    AuthToken `json:"auth_token"`
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

	var response GetThreadsResponse
	err = router.dbManager.View(func(txn *storage.DatabaseTransaction) error {
		// Verify the space exists and belongs to the user
		space, err := txn.GetSpace(getReq.SpaceID)
		if err != nil {
			return errors.Wrap(err, "Failed to get space")
		}
		if space == nil {
			return errors.New("Space not found")
		}
		if space.UserID != userID {
			return errors.New("User does not have permission to view threads in this space")
		}

		if getReq.Limit == 0 {
			getReq.Limit = 10
		}

		if getReq.MaxTimestamp == 0 {
			getReq.MaxTimestamp = uint64(time.Now().UnixNano())
		}

		threads, err := txn.GetSpaceThreads(getReq.SpaceID, getReq.Limit, getReq.MaxTimestamp)
		if err != nil {
			return errors.Wrap(err, "Failed to get threads")
		}

		response.Threads = threads
		if len(threads) > 0 {
			lastThreadTime := uint64(threads[len(threads)-1].UpdatedAt.UnixNano())
			response.NextMaxTimestamp = &lastThreadTime
		}

		return nil
	})

	if err != nil {
		switch {
		case err.Error() == "Space not found":
			http.Error(w, err.Error(), http.StatusNotFound)
		case err.Error() == "User does not have permission to view threads in this space":
			http.Error(w, err.Error(), http.StatusForbidden)
		default:
			http.Error(w, errors.Wrap(err, "Failed to get threads").Error(), http.StatusInternalServerError)
		}
		return
	}

	json.NewEncoder(w).Encode(response)
}

type DeleteThreadRequest struct {
	ThreadID  db.Digest `json:"thread_id"`
	AuthToken AuthToken `json:"auth_token"`
}

type DeleteThreadResponse struct {
	Success bool `json:"success"`
}

func (router *APIRouter) DeleteThread(w http.ResponseWriter, req *http.Request) {
	var deleteReq DeleteThreadRequest
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
		// Get the thread to ensure it exists and get its space ID
		thread, err := txn.GetThread(deleteReq.ThreadID)
		if err != nil {
			return errors.Wrap(err, "Failed to get thread")
		}
		if thread == nil {
			return errors.New("Thread not found")
		}

		// Verify space ownership
		space, err := txn.GetSpace(thread.SpaceID)
		if err != nil {
			return errors.Wrap(err, "Failed to get space")
		}
		if space.UserID != userID {
			return errors.New("User does not have permission to delete this thread")
		}

		if err := txn.DeleteThread(deleteReq.ThreadID); err != nil {
			return errors.Wrap(err, "Failed to delete thread")
		}

		// Update parent space timestamp
		timeNow := time.Now()
		if err := router.updateSpaceWithTxn(txn, thread.SpaceID, userID, space.Name, space.Description, timeNow); err != nil {
			return errors.Wrap(err, "Failed to update parent space")
		}

		return nil
	})

	if err != nil {
		switch {
		case err.Error() == "Thread not found":
			http.Error(w, err.Error(), http.StatusNotFound)
		case err.Error() == "User does not have permission to delete this thread":
			http.Error(w, err.Error(), http.StatusForbidden)
		default:
			http.Error(w, errors.Wrap(err, "Failed to delete thread").Error(), http.StatusInternalServerError)
		}
		return
	}

	response := DeleteThreadResponse{Success: true}
	json.NewEncoder(w).Encode(response)
}

type UpdateThreadRequest struct {
	ThreadID  db.Digest `json:"thread_id"`
	Title     string    `json:"title"`
	AuthToken AuthToken `json:"auth_token"`
}

type UpdateThreadResponse struct {
	Thread storage.Thread `json:"thread"`
}

func (router *APIRouter) UpdateThread(w http.ResponseWriter, req *http.Request) {
	var updateReq UpdateThreadRequest
	if err := json.NewDecoder(req.Body).Decode(&updateReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate the AuthToken
	claims, err := ValidateSessionKey(updateReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	// Get the user ID from the claims
	userID, err := db.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Use a single transaction for the entire update operation
	var updatedThread *storage.Thread
	err = router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		timestamp := time.Now()
		if err := router.updateThreadWithTxn(txn, updateReq.ThreadID, userID, updateReq.Title, timestamp); err != nil {
			return err
		}

		thread, err := txn.GetThread(updateReq.ThreadID)
		if err != nil {
			return errors.Wrap(err, "Failed to get updated thread")
		}
		updatedThread = thread
		return nil
	})

	// Handle errors with appropriate HTTP status codes
	if err != nil {
		switch {
		case err.Error() == "Thread not found":
			http.Error(w, err.Error(), http.StatusNotFound)
		case err.Error() == "Space not found":
			http.Error(w, err.Error(), http.StatusNotFound)
		case err.Error() == "User does not have permission to update this thread":
			http.Error(w, err.Error(), http.StatusForbidden)
		default:
			http.Error(w, errors.Wrap(err, "Failed to update thread").Error(), http.StatusInternalServerError)
		}
		return
	}

	response := UpdateThreadResponse{Thread: *updatedThread}
	json.NewEncoder(w).Encode(response)
}

func (router *APIRouter) updateThreadWithTxn(txn *storage.DatabaseTransaction, threadID db.Digest, userID db.Digest, title string, timestamp time.Time) error {
	thread, err := txn.GetThread(threadID)
	if err != nil {
		return errors.Wrap(err, "Failed to get thread")
	}
	if thread == nil {
		return errors.New("Thread not found")
	}

	space, err := txn.GetSpace(thread.SpaceID)
	if err != nil {
		return errors.Wrap(err, "Failed to get space")
	}
	if space == nil {
		return errors.New("Space not found")
	}
	if space.UserID != userID {
		return errors.New("User does not have permission to update this thread")
	}

	if err := txn.DeleteThread(threadID); err != nil {
		return errors.Wrap(err, "Failed to delete existing thread")
	}

	thread.Title = title
	thread.UpdatedAt = timestamp

	if err := txn.SetThread(thread.SpaceID, thread); err != nil {
		return errors.Wrap(err, "Failed to update thread")
	}

	// Propagate update to parent space with same timestamp
	if err := router.updateSpaceWithTxn(txn, thread.SpaceID, userID, space.Name, space.Description, timestamp); err != nil {
		return errors.Wrap(err, "Failed to update parent space")
	}

	return nil
}

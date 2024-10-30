package api

import (
	"encoding/json"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
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

	spaceIDBytes, err := GenerateRandomBytes(32)
	if err != nil {
		http.Error(w, "Failed to generate space ID", http.StatusInternalServerError)
		return
	}

	var space storage.Space
	err = router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		timeNow := time.Now()
		space = storage.Space{
			ID:          db.NewDigest(spaceIDBytes),
			UserID:      userID,
			Name:        createReq.Name,
			Description: createReq.Description,
			CreatedAt:   timeNow,
			UpdatedAt:   timeNow,
		}

		if err := txn.SetSpace(userID, &space); err != nil {
			return errors.Wrap(err, "Failed to create space")
		}

		return nil
	})

	if err != nil {
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

	var response GetSpacesResponse
	err = router.dbManager.View(func(txn *storage.DatabaseTransaction) error {
		if getReq.Limit == 0 {
			getReq.Limit = 10
		}

		if getReq.MaxTimestamp == 0 {
			getReq.MaxTimestamp = uint64(time.Now().UnixNano())
		}

		spaces, err := txn.GetUserSpaces(userID, getReq.Limit, getReq.MaxTimestamp)
		if err != nil {
			return errors.Wrap(err, "Failed to get spaces")
		}

		response.Spaces = spaces
		if len(spaces) > 0 {
			lastSpaceTime := uint64(spaces[len(spaces)-1].UpdatedAt.UnixNano())
			response.NextMaxTimestamp = &lastSpaceTime
		}

		return nil
	})

	if err != nil {
		http.Error(w, errors.Wrap(err, "Failed to get spaces").Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(response)
}

type DeleteSpaceRequest struct {
	SpaceID   db.Digest `json:"space_id"`
	AuthToken AuthToken `json:"auth_token"`
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
		space, err := txn.GetSpace(deleteReq.SpaceID)
		if err != nil {
			return errors.Wrap(err, "Failed to get space")
		}
		if space == nil {
			return errors.New("Space not found")
		}
		if space.UserID != userID {
			return errors.New("User does not have permission to delete this space")
		}

		if err := txn.DeleteSpace(deleteReq.SpaceID); err != nil {
			return errors.Wrap(err, "Failed to delete space")
		}

		return nil
	})

	if err != nil {
		switch {
		case err.Error() == "Space not found":
			http.Error(w, err.Error(), http.StatusNotFound)
		case err.Error() == "User does not have permission to delete this space":
			http.Error(w, err.Error(), http.StatusForbidden)
		default:
			http.Error(w, errors.Wrap(err, "Failed to delete space").Error(), http.StatusInternalServerError)
		}
		return
	}

	response := DeleteSpaceResponse{Success: true}
	json.NewEncoder(w).Encode(response)
}

type UpdateSpaceRequest struct {
	SpaceID     db.Digest `json:"space_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	AuthToken   AuthToken `json:"auth_token"`
}

type UpdateSpaceResponse struct {
	Space storage.Space `json:"space"`
}

func (router *APIRouter) UpdateSpace(w http.ResponseWriter, req *http.Request) {
	var updateReq UpdateSpaceRequest
	if err := json.NewDecoder(req.Body).Decode(&updateReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	claims, err := ValidateSessionKey(updateReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	userID, err := db.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	var updatedSpace *storage.Space
	err = router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		timestamp := time.Now()
		if err := router.updateSpaceWithTxn(txn, updateReq.SpaceID, userID, updateReq.Name, updateReq.Description, timestamp); err != nil {
			return err
		}

		// Get the fresh copy after update
		space, err := txn.GetSpace(updateReq.SpaceID)
		if err != nil {
			return errors.Wrap(err, "Failed to get updated space")
		}
		updatedSpace = space
		return nil
	})

	if err != nil {
		switch {
		case err.Error() == "Space not found":
			http.Error(w, err.Error(), http.StatusNotFound)
		case err.Error() == "User does not have permission to update this space":
			http.Error(w, err.Error(), http.StatusForbidden)
		default:
			http.Error(w, errors.Wrap(err, "Failed to update space").Error(), http.StatusInternalServerError)
		}
		return
	}

	response := UpdateSpaceResponse{Space: *updatedSpace}
	json.NewEncoder(w).Encode(response)
}

func (router *APIRouter) updateSpaceWithTxn(txn *storage.DatabaseTransaction, spaceID db.Digest, userID db.Digest, name string, description string, timestamp time.Time) error {
	space, err := txn.GetSpace(spaceID)
	if err != nil {
		return errors.Wrap(err, "Failed to get space")
	}
	if space == nil {
		return errors.New("Space not found")
	}
	if space.UserID != userID {
		return errors.New("User does not have permission to update this space")
	}

	if err := txn.DeleteSpace(spaceID); err != nil {
		return errors.Wrap(err, "Failed to delete existing space")
	}

	space.Name = name
	space.Description = description
	space.UpdatedAt = timestamp

	if err := txn.SetSpace(space.UserID, space); err != nil {
		return errors.Wrap(err, "Failed to update space")
	}

	return nil
}

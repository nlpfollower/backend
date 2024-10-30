package api

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/pkg/errors"
	"net/http"
	"time"
)

type SignUpRequest struct {
	Email       string             `json:"email"`
	Username    string             `json:"username"`
	PasswordHex string             `json:"password_hex"`
	Method      storage.AuthMethod `json:"method"`
}

type SignUpResponse struct {
	AuthToken AuthToken `json:"auth_token"`
}

func (router *APIRouter) SignUp(w http.ResponseWriter, r *http.Request) {
	var req SignUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	switch req.Method {
	case storage.AuthMethodEmailPassword:
		if err := router.signUpEmailPassword(w, req); err != nil {
			switch {
			case err == ErrUserAlreadyExists:
				http.Error(w, err.Error(), http.StatusConflict)
			default:
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}
	case storage.AuthMethodEmailLink:
		http.Error(w, "Email link authentication not implemented yet", http.StatusNotImplemented)
	case storage.AuthMethodGmail:
		http.Error(w, "Gmail authentication not implemented yet", http.StatusNotImplemented)
	default:
		http.Error(w, "Invalid authentication method", http.StatusBadRequest)
	}
}

func (router *APIRouter) signUpEmailPassword(w http.ResponseWriter, req SignUpRequest) error {
	userID := db.NewDigest(GetUserIDFromEmail(req.Email))

	var user *storage.User
	err := router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		existingUser, err := txn.GetUser(userID)
		if err != nil {
			return errors.Wrap(err, "Failed to check existing user")
		}
		if existingUser != nil {
			return ErrUserAlreadyExists
		}

		passwordHash, err := hex.DecodeString(req.PasswordHex)
		if err != nil {
			return errors.Wrap(err, "Invalid password format")
		}

		seed, err := GenerateRandomSeed()
		if err != nil {
			return errors.Wrap(err, "Failed to generate seed")
		}

		encryptedSeed, err := EncryptSeed(seed, passwordHash)
		if err != nil {
			return errors.Wrap(err, "Failed to encrypt seed")
		}

		user = &storage.User{
			ID:            userID,
			Email:         req.Email,
			Username:      req.Username,
			PasswordHash:  passwordHash,
			EncryptedSeed: encryptedSeed,
			AuthMethod:    req.Method,
			CreatedAt:     time.Now(),
		}

		if err := txn.SetUser(user); err != nil {
			return errors.Wrap(err, "Failed to create user")
		}

		return nil
	})

	if err != nil {
		return err
	}

	authToken, err := GenerateAuthToken(user.ID.String())
	if err != nil {
		return errors.Wrap(err, "Failed to generate auth token")
	}

	response := SignUpResponse{
		AuthToken: *authToken,
	}
	json.NewEncoder(w).Encode(response)
	return nil
}

type SignInRequest struct {
	Email       string `json:"email"`
	PasswordHex string `json:"password_hex"`
}

type SignInResponse struct {
	UserID    string    `json:"user_id"`
	AuthToken AuthToken `json:"auth_token"`
}

func (router *APIRouter) SignIn(w http.ResponseWriter, r *http.Request) {
	var req SignInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	userID := db.NewDigest(GetUserIDFromEmail(req.Email))

	var response SignInResponse
	err := router.dbManager.View(func(txn *storage.DatabaseTransaction) error {
		user, err := txn.GetUser(userID)
		if err != nil {
			return errors.Wrap(err, "Failed to get user")
		}
		if user == nil {
			return ErrInvalidCredentials
		}

		passwordHash, err := hex.DecodeString(req.PasswordHex)
		if err != nil {
			return errors.Wrap(err, "Invalid password format")
		}

		if !comparePasswordHashes(user.PasswordHash, passwordHash) {
			return ErrInvalidCredentials
		}

		authToken, err := GenerateAuthToken(user.ID.String())
		if err != nil {
			return errors.Wrap(err, "Failed to generate auth token")
		}

		response = SignInResponse{
			UserID:    user.ID.String(),
			AuthToken: *authToken,
		}

		return nil
	})

	if err != nil {
		switch {
		case err == ErrInvalidCredentials:
			http.Error(w, err.Error(), http.StatusUnauthorized)
		default:
			http.Error(w, errors.Wrap(err, "Failed to sign in").Error(), http.StatusInternalServerError)
		}
		return
	}

	json.NewEncoder(w).Encode(response)
}

func comparePasswordHashes(stored, provided []byte) bool {
	if len(stored) != len(provided) {
		return false
	}
	return subtle.ConstantTimeCompare(stored, provided) == 1
}

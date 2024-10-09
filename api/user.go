package api

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"github.com/nlpfollower/deltamind/backend/storage"
	"net/http"
	"time"
)

type SignUpRequest struct {
	Email       string             `json:"email"`
	Username    string             `json:"username"`
	PasswordHex string             `json:"password"`
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
			if err == ErrUserAlreadyExists {
				http.Error(w, err.Error(), http.StatusConflict)
			} else {
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
	// Check if user already exists
	userID := storage.NewDigest(GetUserIDFromEmail(req.Email))
	existingUser, err := router.dbManager.GetUser(userID)
	if err != nil {
		return err
	}
	if existingUser != nil {
		return ErrUserAlreadyExists
	}

	passwordHash, err := hex.DecodeString(req.PasswordHex)
	if err != nil {
		return err
	}

	seed, err := GenerateRandomSeed()
	if err != nil {
		return err
	}

	encryptedSeed, err := EncryptSeed(seed, passwordHash)
	if err != nil {
		return err
	}

	user := &storage.User{
		ID:            storage.NewDigest(GetUserIDFromEmail(req.Email)),
		Email:         req.Email,
		Username:      req.Username,
		PasswordHash:  passwordHash,
		EncryptedSeed: encryptedSeed,
		AuthMethod:    req.Method,
		CreatedAt:     time.Now(),
	}

	if err := router.dbManager.CreateUser(user); err != nil {
		return err
	}

	authToken, err := GenerateAuthToken(user.ID.String())
	if err != nil {
		return err
	}

	response := SignUpResponse{
		AuthToken: *authToken,
	}
	json.NewEncoder(w).Encode(response)

	return nil
}

type SignInRequest struct {
	Email       string `json:"email"`
	PasswordHex string `json:"password"`
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

	userID := storage.NewDigest(GetUserIDFromEmail(req.Email))
	user, err := router.dbManager.GetUser(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, ErrInvalidCredentials.Error(), http.StatusUnauthorized)
		return
	}

	passwordHash, err := hex.DecodeString(req.PasswordHex)
	if err != nil {
		http.Error(w, "Invalid password format", http.StatusBadRequest)
		return
	}

	if !comparePasswordHashes(user.PasswordHash, passwordHash) {
		http.Error(w, ErrInvalidCredentials.Error(), http.StatusUnauthorized)
		return
	}

	authToken, err := GenerateAuthToken(user.ID.String())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := SignInResponse{
		UserID:    user.ID.String(),
		AuthToken: *authToken,
	}
	json.NewEncoder(w).Encode(response)
}

func comparePasswordHashes(stored, provided []byte) bool {
	if len(stored) != len(provided) {
		return false
	}
	return subtle.ConstantTimeCompare(stored, provided) == 1
}

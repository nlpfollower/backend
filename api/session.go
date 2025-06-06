package api

import (
	"encoding/json"
	"fmt"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/nlpfollower/deltamind/nexus/core"
	"net/http"
	"time"
)

// Session management types matching nexus types
type SessionAction string

const (
	SessionActionStart  SessionAction = "START"
	SessionActionStop   SessionAction = "STOP"
	SessionActionExtend SessionAction = "EXTEND"
)

type SessionRequest struct {
	Action    SessionAction `json:"action"`
	ModelID   string        `json:"model_id,omitempty"`   // Required for START
	SessionID string        `json:"session_id,omitempty"` // Required for STOP and EXTEND
	Duration  string        `json:"duration,omitempty"`   // Optional for EXTEND, format like "30m"
	AuthToken AuthToken     `json:"auth_token"`
}

type SessionResponse struct {
	Status    string `json:"status"`
	SessionID string `json:"session_id,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	Error     string `json:"error,omitempty"`
}

// HTTP endpoint for session management
func (router *APIRouter) ManageSession(w http.ResponseWriter, req *http.Request) {
	var sessionReq SessionRequest
	if err := json.NewDecoder(req.Body).Decode(&sessionReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate auth token
	claims, err := ValidateSessionKey(sessionReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	userID, err := db.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Generate request ID
	requestIDBytes, err := GenerateRandomBytes(32)
	if err != nil {
		http.Error(w, "Failed to generate request ID", http.StatusInternalServerError)
		return
	}
	requestID := db.NewDigest(requestIDBytes)

	// Create nexus session request
	nexusSessionReq := &core.SessionRequest{
		Action:    core.SessionAction(sessionReq.Action),
		ModelID:   sessionReq.ModelID,
		SessionID: sessionReq.SessionID,
		Duration:  sessionReq.Duration,
	}

	wrappedReq, err := core.NewWrappedRequest(requestID, nexusSessionReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create request: %v", err), http.StatusInternalServerError)
		return
	}

	// Send to nexus and wait for response
	responseChan, err := router.nexusClient.EnqueueSession(userID, nexusSessionReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to enqueue session request: %v", err), http.StatusInternalServerError)
		return
	}

	// Wait for response with timeout
	select {
	case nexusResp := <-responseChan:
		var sessionResp core.SessionResponse
		if err := json.Unmarshal(nexusResp.Data, &sessionResp); err != nil {
			http.Error(w, fmt.Sprintf("Failed to parse session response: %v", err), http.StatusInternalServerError)
			return
		}

		// Convert to API response
		apiResp := SessionResponse{
			Status:    string(sessionResp.Status),
			SessionID: sessionResp.SessionID,
			Endpoint:  sessionResp.Endpoint,
			Error:     sessionResp.Error,
		}

		w.Header().Set("Content-Type", "application/json")
		if sessionResp.Status == core.ResponseStatusError {
			w.WriteHeader(http.StatusBadRequest)
		}
		json.NewEncoder(w).Encode(apiResp)

	case <-time.After(30 * time.Second):
		http.Error(w, "Session request timeout", http.StatusRequestTimeout)
	}
}

// WebSocket message types for session management
type WSSessionRequest struct {
	Action    SessionAction `json:"action"`
	ModelID   string        `json:"model_id,omitempty"`
	SessionID string        `json:"session_id,omitempty"`
	Duration  string        `json:"duration,omitempty"`
	AuthToken AuthToken     `json:"auth_token"`
}

func (s WSSessionRequest) GetWSMessageType() WSMessageType {
	return WSMessageTypeSession
}

type WSSessionResponse struct {
	Status    string `json:"status"`
	SessionID string `json:"session_id,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s WSSessionResponse) GetWSMessageType() WSMessageType {
	return WSMessageTypeSession
}

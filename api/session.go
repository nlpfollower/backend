package api

import (
	"encoding/json"
	"fmt"
	nexusCore "github.com/nlpfollower/deltamind/nexus/core"
	"net/http"
)

// StartSessionRequest contains the request data for starting a session
type StartSessionRequest struct {
	ModelID   string    `json:"model_id"`
	AuthToken AuthToken `json:"auth_token"`
}

// StartSessionResponse contains the response after starting a session
type StartSessionResponse struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
}

// SessionStatusRequest contains the request data for checking session status
type SessionStatusRequest struct {
	SessionID string    `json:"session_id"`
	AuthToken AuthToken `json:"auth_token"`
}

// SessionStatusResponse contains the current status of a session
type SessionStatusResponse struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	Endpoint  string `json:"endpoint,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ExtendSessionRequest contains the request data for extending a session
type ExtendSessionRequest struct {
	SessionID string    `json:"session_id"`
	Duration  string    `json:"duration"` // e.g., "30m", "1h"
	AuthToken AuthToken `json:"auth_token"`
}

// ExtendSessionResponse contains the response after extending a session
type ExtendSessionResponse struct {
	Success bool `json:"success"`
}

// StopSessionRequest contains the request data for stopping a session
type StopSessionRequest struct {
	SessionID string    `json:"session_id"`
	AuthToken AuthToken `json:"auth_token"`
}

// StopSessionResponse contains the response after stopping a session
type StopSessionResponse struct {
	Success bool `json:"success"`
}

// StartSession starts a new inference session asynchronously
func (router *APIRouter) StartSession(w http.ResponseWriter, req *http.Request) {
	var startReq StartSessionRequest
	if err := json.NewDecoder(req.Body).Decode(&startReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate auth token
	_, err := ValidateSessionKey(startReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	// Check if user has permission to use this model
	// TODO: Add model permission checks here if needed

	// Send session start request to Nexus
	sessionReq := &nexusCore.SessionRequest{
		Action:  nexusCore.SessionActionStart,
		ModelID: startReq.ModelID,
	}

	respChan, err := router.nexusClient.EnqueueSession(sessionReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start session: %v", err), http.StatusInternalServerError)
		return
	}

	// Wait for response
	resp, ok := <-respChan
	if !ok {
		http.Error(w, "Failed to receive session response", http.StatusInternalServerError)
		return
	}

	// Parse session response
	var sessionResp nexusCore.SessionResponse
	if err := json.Unmarshal(resp.Data, &sessionResp); err != nil {
		http.Error(w, "Failed to parse session response", http.StatusInternalServerError)
		return
	}

	if sessionResp.Status != nexusCore.ResponseStatusSuccess {
		http.Error(w, fmt.Sprintf("Failed to start session: %s", sessionResp.Error), http.StatusInternalServerError)
		return
	}

	// Return session ID and initial state
	response := StartSessionResponse{
		SessionID: sessionResp.SessionID,
		State:     sessionResp.State,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetSessionStatus checks the current status of a session
func (router *APIRouter) GetSessionStatus(w http.ResponseWriter, req *http.Request) {
	var statusReq SessionStatusRequest
	if err := json.NewDecoder(req.Body).Decode(&statusReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate auth token
	_, err := ValidateSessionKey(statusReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	// Send status check request to Nexus
	sessionReq := &nexusCore.SessionRequest{
		Action:    nexusCore.SessionActionStatus,
		SessionID: statusReq.SessionID,
	}

	respChan, err := router.nexusClient.EnqueueSession(sessionReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to check session status: %v", err), http.StatusInternalServerError)
		return
	}

	// Wait for response
	resp, ok := <-respChan
	if !ok {
		http.Error(w, "Failed to receive session response", http.StatusInternalServerError)
		return
	}

	// Parse session response
	var sessionResp nexusCore.SessionResponse
	if err := json.Unmarshal(resp.Data, &sessionResp); err != nil {
		http.Error(w, "Failed to parse session response", http.StatusInternalServerError)
		return
	}

	if sessionResp.Status != nexusCore.ResponseStatusSuccess {
		http.Error(w, fmt.Sprintf("Failed to get session status: %s", sessionResp.Error), http.StatusInternalServerError)
		return
	}

	// Return session status
	response := SessionStatusResponse{
		SessionID: statusReq.SessionID,
		State:     sessionResp.State,
		Endpoint:  sessionResp.Endpoint,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ExtendSession extends the lifetime of an existing session
func (router *APIRouter) ExtendSession(w http.ResponseWriter, req *http.Request) {
	var extendReq ExtendSessionRequest
	if err := json.NewDecoder(req.Body).Decode(&extendReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate auth token
	_, err := ValidateSessionKey(extendReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	// Send extend request to Nexus
	sessionReq := &nexusCore.SessionRequest{
		Action:    nexusCore.SessionActionExtend,
		SessionID: extendReq.SessionID,
		Duration:  extendReq.Duration,
	}

	respChan, err := router.nexusClient.EnqueueSession(sessionReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to extend session: %v", err), http.StatusInternalServerError)
		return
	}

	// Wait for response
	resp, ok := <-respChan
	if !ok {
		http.Error(w, "Failed to receive session response", http.StatusInternalServerError)
		return
	}

	// Parse session response
	var sessionResp nexusCore.SessionResponse
	if err := json.Unmarshal(resp.Data, &sessionResp); err != nil {
		http.Error(w, "Failed to parse session response", http.StatusInternalServerError)
		return
	}

	if sessionResp.Status != nexusCore.ResponseStatusSuccess {
		http.Error(w, fmt.Sprintf("Failed to extend session: %s", sessionResp.Error), http.StatusInternalServerError)
		return
	}

	response := ExtendSessionResponse{Success: true}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// StopSession stops an existing session
func (router *APIRouter) StopSession(w http.ResponseWriter, req *http.Request) {
	var stopReq StopSessionRequest
	if err := json.NewDecoder(req.Body).Decode(&stopReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate auth token
	_, err := ValidateSessionKey(stopReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	// Send stop request to Nexus
	sessionReq := &nexusCore.SessionRequest{
		Action:    nexusCore.SessionActionStop,
		SessionID: stopReq.SessionID,
	}

	respChan, err := router.nexusClient.EnqueueSession(sessionReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop session: %v", err), http.StatusInternalServerError)
		return
	}

	// Wait for response - the Nexus should respond immediately
	// The actual stop operation happens in the background
	resp, ok := <-respChan
	if !ok {
		http.Error(w, "Failed to receive session response", http.StatusInternalServerError)
		return
	}

	// Parse session response
	var sessionResp nexusCore.SessionResponse
	if err := json.Unmarshal(resp.Data, &sessionResp); err != nil {
		http.Error(w, "Failed to parse session response", http.StatusInternalServerError)
		return
	}

	if sessionResp.Status != nexusCore.ResponseStatusSuccess {
		http.Error(w, fmt.Sprintf("Failed to stop session: %s", sessionResp.Error), http.StatusInternalServerError)
		return
	}

	response := StopSessionResponse{Success: true}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

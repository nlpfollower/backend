package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/stretchr/testify/require"
)

func TestSessionManagement(t *testing.T) {
	// Skip if not using real Nexus
	if os.Getenv("USE_REAL_NEXUS") != "true" {
		t.Skip("Skipping session management test. Set USE_REAL_NEXUS=true to run.")
	}

	// Use real Nexus server
	baseURL := "http://localhost:8080/api/v0"
	t.Log("Using real Nexus server for session tests")

	// First sign in to get auth token
	authToken := signInForTests(t, baseURL)

	t.Run("Session Lifecycle", func(t *testing.T) {
		client := &http.Client{Timeout: 30 * time.Second}

		// Start session
		startReq := StartSessionRequest{
			ModelID:   "llama-8b",
			AuthToken: authToken,
		}

		reqBody, _ := json.Marshal(startReq)
		resp, err := client.Post(baseURL+"/session/start", "application/json", bytes.NewBuffer(reqBody))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var startResp StartSessionResponse
		err = json.NewDecoder(resp.Body).Decode(&startResp)
		resp.Body.Close()
		require.NoError(t, err)
		require.NotEmpty(t, startResp.SessionID)

		sessionID := startResp.SessionID
		t.Logf("Session %s started with initial state: %s", sessionID, startResp.State)

		// Poll for session readiness
		var sessionEndpoint string
		deadline := time.Now().Add(10 * time.Minute)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		pollCount := 0
		for time.Now().Before(deadline) {
			pollCount++

			// Check session status
			statusReq := SessionStatusRequest{
				SessionID: sessionID,
				AuthToken: authToken,
			}

			reqBody, _ := json.Marshal(statusReq)
			statusResp, err := client.Post(baseURL+"/session/status", "application/json", bytes.NewBuffer(reqBody))
			if err != nil {
				t.Logf("Poll %d: Error checking status: %v", pollCount, err)
				<-ticker.C
				continue
			}

			var status SessionStatusResponse
			err = json.NewDecoder(statusResp.Body).Decode(&status)
			statusResp.Body.Close()

			if err != nil {
				t.Logf("Poll %d: Error decoding status: %v", pollCount, err)
				<-ticker.C
				continue
			}

			t.Logf("Poll %d: Session state = %s", pollCount, status.State)

			// Check if session is ready
			if status.State == "running" {
				sessionEndpoint = status.Endpoint
				t.Logf("Session ready after %d polls! Endpoint: %s", pollCount, sessionEndpoint)
				break
			}

			if status.State == "error" || status.State == "stopped" || status.State == "expired" {
				t.Fatalf("Session failed to start: state = %s, error = %s", status.State, status.Error)
			}

			<-ticker.C
		}

		require.NotEmpty(t, sessionEndpoint, "Session should have an endpoint after initialization")

		// Test extending session
		extendReq := ExtendSessionRequest{
			SessionID: sessionID,
			Duration:  "1h",
			AuthToken: authToken,
		}

		reqBody, _ = json.Marshal(extendReq)
		resp, err = client.Post(baseURL+"/session/extend", "application/json", bytes.NewBuffer(reqBody))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var extendResp ExtendSessionResponse
		err = json.NewDecoder(resp.Body).Decode(&extendResp)
		resp.Body.Close()
		require.NoError(t, err)
		require.True(t, extendResp.Success)
		t.Log("Session extended successfully")

		// Stop session
		stopReq := StopSessionRequest{
			SessionID: sessionID,
			AuthToken: authToken,
		}

		reqBody, _ = json.Marshal(stopReq)
		resp, err = client.Post(baseURL+"/session/stop", "application/json", bytes.NewBuffer(reqBody))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var stopResp StopSessionResponse
		err = json.NewDecoder(resp.Body).Decode(&stopResp)
		resp.Body.Close()
		require.NoError(t, err)
		require.True(t, stopResp.Success)
		t.Log("Session stopped successfully")

		// Verify session is stopped
		statusReq := SessionStatusRequest{
			SessionID: sessionID,
			AuthToken: authToken,
		}

		reqBody, _ = json.Marshal(statusReq)
		finalStatusResp, err := client.Post(baseURL+"/session/status", "application/json", bytes.NewBuffer(reqBody))
		require.NoError(t, err)

		var finalStatus SessionStatusResponse
		err = json.NewDecoder(finalStatusResp.Body).Decode(&finalStatus)
		finalStatusResp.Body.Close()
		require.NoError(t, err)
		require.Contains(t, []string{"stopped", "stopping"}, finalStatus.State)

		t.Log("Session lifecycle test completed successfully")
	})
}

// Helper function to sign in and get auth token for tests
func signInForTests(t *testing.T, baseURL string) AuthToken {
	client := &http.Client{Timeout: 10 * time.Second}

	// Try to sign up first (in case user doesn't exist)
	passwordHex := hex.EncodeToString(HashPassword("testpassword123"))
	signUpReq := SignUpRequest{
		Email:       "test@example.com",
		Username:    "testuser",
		PasswordHex: passwordHex,
		Method:      storage.AuthMethodEmailPassword,
	}

	reqBody, _ := json.Marshal(signUpReq)
	resp, err := client.Post(baseURL+"/sign-up", "application/json", bytes.NewBuffer(reqBody))
	if err == nil {
		resp.Body.Close()
	}

	// Sign in
	signInReq := SignInRequest{
		Email:       "test@example.com",
		PasswordHex: passwordHex,
	}

	reqBody, _ = json.Marshal(signInReq)
	resp, err = client.Post(baseURL+"/sign-in", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var signInResp SignInResponse
	err = json.NewDecoder(resp.Body).Decode(&signInResp)
	resp.Body.Close()
	require.NoError(t, err)

	return AuthToken{
		SessionKey: signInResp.AuthToken.SessionKey,
	}
}

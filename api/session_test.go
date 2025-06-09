package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
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

	t.Run("Session Lifecycle with WebSocket Inference", func(t *testing.T) {
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

		// Test sending an inference request via WebSocket
		t.Log("Testing WebSocket inference request...")

		// Create test user data for WebSocket inference
		user, err := createTestUserForSession(t, baseURL, authToken)
		require.NoError(t, err)

		// Connect to WebSocket
		origin := "http://localhost/"
		wsURL := fmt.Sprintf("ws://localhost:8080/ws")
		ws, err := websocket.Dial(wsURL, "", origin)
		require.NoError(t, err)
		defer ws.Close()

		// Send WebSocket handshake
		handshakeReq := HandshakeRequest{
			AuthToken: authToken,
		}
		err = sendTypedWSMessage(ws, handshakeReq)
		require.NoError(t, err)

		// Read handshake response
		handshakeResp, err := receiveTypedWSMessage[HandshakeResponse](ws)
		require.NoError(t, err)
		require.Equal(t, "success", handshakeResp.Status)
		t.Log("WebSocket handshake successful")

		// Send inference request via WebSocket
		inferReq := WSInferenceRequest{
			LastMessageID: &user.LastMessageID,
			ModelID:       "llama-8b",
			AuthToken:     authToken,
		}
		err = sendTypedWSMessage(ws, inferReq)
		require.NoError(t, err)
		t.Log("Sent WebSocket inference request")

		// Read inference responses
		receivedContent := false
		receivedFinal := false
		timeout := time.After(60 * time.Second)

		for !receivedFinal {
			select {
			case <-timeout:
				t.Fatal("Timeout waiting for inference response")
			default:
				inferResp, err := receiveTypedWSMessage[WSInferenceResponse](ws)
				require.NoError(t, err)

				if inferResp.Status == "error" {
					t.Fatalf("Inference error: %s", inferResp.Content)
				}

				if inferResp.Content != "" {
					receivedContent = true
					t.Logf("Received inference content: %q", inferResp.Content)
				}

				if inferResp.Type == "final" {
					receivedFinal = true
					t.Log("Received final inference response")
				}
			}
		}

		require.True(t, receivedContent, "Should have received some content from inference")
		ws.Close()

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

		// Stop session - this should respond immediately
		t.Log("Stopping session...")
		stopReq := StopSessionRequest{
			SessionID: sessionID,
			AuthToken: authToken,
		}

		reqBody, _ = json.Marshal(stopReq)
		stopStartTime := time.Now()
		resp, err = client.Post(baseURL+"/session/stop", "application/json", bytes.NewBuffer(reqBody))
		stopDuration := time.Since(stopStartTime)

		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var stopResp StopSessionResponse
		err = json.NewDecoder(resp.Body).Decode(&stopResp)
		resp.Body.Close()
		require.NoError(t, err)
		require.True(t, stopResp.Success)

		// Ensure stop responded quickly (should be under 1 second)
		require.Less(t, stopDuration, 1*time.Second, "Stop request should respond immediately")
		t.Logf("Session stop request completed in %v", stopDuration)

		// Poll status to verify session is stopping/stopped
		stoppedDeadline := time.Now().Add(2 * time.Minute)
		ticker2 := time.NewTicker(2 * time.Second)
		defer ticker2.Stop()

		checkCount := 0
		for time.Now().Before(stoppedDeadline) {
			checkCount++

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

			t.Logf("Stop poll %d: Session state = %s", checkCount, finalStatus.State)

			// Accept stopping, stopped, or error states
			if finalStatus.State == "stopped" {
				t.Log("Session successfully stopped")
				break
			}

			if finalStatus.State == "error" {
				t.Logf("Session stopped with error state")
				break
			}

			// Keep checking if still stopping
			if finalStatus.State == "stopping" {
				t.Logf("Session is still stopping...")
			}

			<-ticker2.C
		}

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

// Helper function to create test user data for WebSocket inference
type TestUserData struct {
	AuthToken     AuthToken
	LastMessageID storage.CompoundMessageID
}

func createTestUserForSession(t *testing.T, baseURL string, authToken AuthToken) (*TestUserData, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Create a space
	createSpaceReq := map[string]interface{}{
		"name":        "Test Space for Session",
		"description": "Test space for session management",
		"auth_token":  authToken,
	}

	reqBody, _ := json.Marshal(createSpaceReq)
	resp, err := client.Post(baseURL+"/space/create", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create space: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create space: %s - %s", resp.Status, string(body))
	}

	var spaceResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&spaceResp); err != nil {
		return nil, fmt.Errorf("failed to decode space response: %w", err)
	}

	spaceID, ok := spaceResp["space_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid space_id in response")
	}

	// Create a thread
	createThreadReq := map[string]interface{}{
		"space_id":    spaceID,
		"name":        "Test Thread for Session",
		"description": "Test thread for session management",
		"auth_token":  authToken,
	}

	reqBody, _ = json.Marshal(createThreadReq)
	resp, err = client.Post(baseURL+"/thread/create", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create thread: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create thread: %s - %s", resp.Status, string(body))
	}

	var threadResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&threadResp); err != nil {
		return nil, fmt.Errorf("failed to decode thread response: %w", err)
	}

	threadID, ok := threadResp["thread_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid thread_id in response")
	}

	// Create a message
	createMessageReq := map[string]interface{}{
		"thread_id":  threadID,
		"author":     "user",
		"messages":   []string{"Hello, this is a test message for session inference."},
		"auth_token": authToken,
	}

	reqBody, _ = json.Marshal(createMessageReq)
	resp, err = client.Post(baseURL+"/message/create", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create message: %s - %s", resp.Status, string(body))
	}

	var messageResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&messageResp); err != nil {
		return nil, fmt.Errorf("failed to decode message response: %w", err)
	}

	messageIDStr, ok := messageResp["message_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid message_id in response")
	}

	// Convert string message ID to digest
	messageIDBytes, err := hex.DecodeString(messageIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode message ID: %w", err)
	}

	var messageIDArray [32]byte
	copy(messageIDArray[:], messageIDBytes)

	messageID := storage.CompoundMessageID{
		ID:        messageIDArray,
		MessageID: 0,
	}

	return &TestUserData{
		AuthToken:     authToken,
		LastMessageID: messageID,
	}, nil
}

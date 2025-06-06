// api/session_test.go
package api

import (
	"fmt"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestSessionManagement(t *testing.T) {
	// Check if we should use real Nexus server
	useRealNexus := os.Getenv("USE_REAL_NEXUS") == "true"
	nexusPort := 8081 // Default Nexus port

	if useRealNexus {
		t.Log("Using real Nexus server on port", nexusPort)
		// Optionally check if Nexus is running
		// You could add a health check here
	} else {
		t.Skip("Skipping session test. Set USE_REAL_NEXUS=true and ensure Nexus is running on port 8081")
	}

	// Create test server that connects to real Nexus
	tempDir, err := os.MkdirTemp("", "deltamind-session-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	server, err := NewServer(tempDir, nexusPort)
	require.NoError(t, err)

	// Start the NexusClient
	server.nexusClient.Start()
	defer server.nexusClient.Stop()

	// Use httptest.NewServer to create a test server
	httpServer := httptest.NewServer(server.router)
	defer httpServer.Close()

	// Create WebSocket URL
	wsURL := url.URL{Scheme: "ws", Host: httpServer.Listener.Addr().String(), Path: "/ws"}

	ts := &TestServer{
		Server:     server,
		URL:        httpServer.URL,
		WsURL:      wsURL.String(),
		httpServer: httpServer,
	}

	// Wait for the NexusClient to establish a connection
	require.NoError(t, waitForNexusConnection(server.nexusClient, 5*time.Second))

	t.Run("HTTP Session Management", func(t *testing.T) {
		// Create test user
		user, err := createTestUser(t, ts, "session_user@example.com", "sessionuser", "password123")
		require.NoError(t, err)

		// Test START session
		startReq := SessionRequest{
			Action:    SessionActionStart,
			ModelID:   "llama-8b",
			AuthToken: user.AuthToken,
		}

		startResp, err := performRequest[SessionRequest, SessionResponse](t, ts, "POST", "/api/v0/manage-session", startReq)
		require.NoError(t, err)
		require.Equal(t, "SUCCESS", startResp.Status)
		require.NotEmpty(t, startResp.SessionID)
		require.NotEmpty(t, startResp.Endpoint)

		sessionID := startResp.SessionID
		t.Logf("Started session: %s at endpoint: %s", sessionID, startResp.Endpoint)

		// Test EXTEND session
		extendReq := SessionRequest{
			Action:    SessionActionExtend,
			SessionID: sessionID,
			Duration:  "1h",
			AuthToken: user.AuthToken,
		}

		extendResp, err := performRequest[SessionRequest, SessionResponse](t, ts, "POST", "/api/v0/manage-session", extendReq)
		require.NoError(t, err)
		require.Equal(t, "SUCCESS", extendResp.Status)
		require.Equal(t, sessionID, extendResp.SessionID)
		t.Log("Extended session successfully")

		// Test STOP session
		stopReq := SessionRequest{
			Action:    SessionActionStop,
			SessionID: sessionID,
			AuthToken: user.AuthToken,
		}

		stopResp, err := performRequest[SessionRequest, SessionResponse](t, ts, "POST", "/api/v0/manage-session", stopReq)
		require.NoError(t, err)
		require.Equal(t, "SUCCESS", stopResp.Status)
		t.Log("Stopped session successfully")
	})

	t.Run("WebSocket Session Management", func(t *testing.T) {
		user, err := createTestUser(t, ts, "ws_session@example.com", "wssessionuser", "password123")
		require.NoError(t, err)

		origin := "http://localhost/"
		url := fmt.Sprintf("ws://%s/ws", ts.URL[7:])
		ws, err := websocket.Dial(url, "", origin)
		require.NoError(t, err)
		defer ws.Close()

		// Send handshake
		handshakeReq := HandshakeRequest{
			AuthToken: user.AuthToken,
		}
		err = sendTypedWSMessage(ws, handshakeReq)
		require.NoError(t, err)

		// Read handshake response
		handshakeResp, err := receiveTypedWSMessage[HandshakeResponse](ws)
		require.NoError(t, err)
		require.Equal(t, "success", handshakeResp.Status)

		// Send START session request
		sessionReq := WSSessionRequest{
			Action:    SessionActionStart,
			ModelID:   "llama-8b",
			AuthToken: user.AuthToken,
		}
		err = sendTypedWSMessage(ws, sessionReq)
		require.NoError(t, err)

		// Read session response
		sessionResp, err := receiveTypedWSMessage[WSSessionResponse](ws)
		require.NoError(t, err)
		require.Equal(t, "SUCCESS", sessionResp.Status)
		require.NotEmpty(t, sessionResp.SessionID)
		require.NotEmpty(t, sessionResp.Endpoint)
		t.Logf("WebSocket session started: %s at endpoint: %s", sessionResp.SessionID, sessionResp.Endpoint)
	})

	t.Run("Session and Inference Integration", func(t *testing.T) {
		user, err := createTestUser(t, ts, "integration@example.com", "integrationuser", "password123")
		require.NoError(t, err)

		// Create test data
		space := createTestSpace(t, ts, user)
		thread := createTestThread(t, ts, user, space, "Test Thread")
		messages := createTestMessages(t, ts, user, thread, 1)
		require.NotEmpty(t, messages)
		lastMessage := messages[0]

		origin := "http://localhost/"
		url := fmt.Sprintf("ws://%s/ws", ts.URL[7:])
		ws, err := websocket.Dial(url, "", origin)
		require.NoError(t, err)
		defer ws.Close()

		// Handshake
		handshakeReq := HandshakeRequest{
			AuthToken: user.AuthToken,
		}
		err = sendTypedWSMessage(ws, handshakeReq)
		require.NoError(t, err)

		handshakeResp, err := receiveTypedWSMessage[HandshakeResponse](ws)
		require.NoError(t, err)
		require.Equal(t, "success", handshakeResp.Status)

		// Start session (optional - can also just send inference directly)
		sessionReq := WSSessionRequest{
			Action:    SessionActionStart,
			ModelID:   "llama-8b",
			AuthToken: user.AuthToken,
		}
		err = sendTypedWSMessage(ws, sessionReq)
		require.NoError(t, err)

		sessionResp, err := receiveTypedWSMessage[WSSessionResponse](ws)
		require.NoError(t, err)
		require.Equal(t, "SUCCESS", sessionResp.Status)
		sessionID := sessionResp.SessionID
		t.Logf("Session started: %s", sessionID)

		// Now send inference request using API model (GPT-4)
		inferReq := WSInferenceRequest{
			LastMessageID: &storage.CompoundMessageID{
				ID:        lastMessage.ID,
				MessageID: 0,
			},
			ModelID:   "llama-8b",
			AuthToken: user.AuthToken,
		}
		err = sendTypedWSMessage(ws, inferReq)
		require.NoError(t, err)

		// Read inference responses (might be multiple for streaming)
		var receivedContent string
		for {
			inferResp, err := receiveTypedWSMessage[WSInferenceResponse](ws)
			require.NoError(t, err)

			if inferResp.Status == "error" {
				t.Fatalf("Inference error: %s", inferResp.Content)
			}

			receivedContent += inferResp.Content
			t.Logf("Received inference response (type=%s): %s", inferResp.Type, inferResp.Content)

			if inferResp.Type == "FINAL" {
				break
			}
		}

		require.NotEmpty(t, receivedContent)
		t.Logf("Full inference response: %s", receivedContent)

		// Stop session
		stopReq := WSSessionRequest{
			Action:    SessionActionStop,
			SessionID: sessionID,
			AuthToken: user.AuthToken,
		}
		err = sendTypedWSMessage(ws, stopReq)
		require.NoError(t, err)

		stopResp, err := receiveTypedWSMessage[WSSessionResponse](ws)
		require.NoError(t, err)
		require.Equal(t, "SUCCESS", stopResp.Status)
		t.Log("Session stopped successfully")
	})
}

// Helper function to wait for nexus connection
func waitForNexusConnection(client *NexusClient, timeout time.Duration) error {
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for Nexus connection")
		}

		if client.isConnected {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}
}

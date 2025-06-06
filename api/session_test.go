// api/session_test.go
package api

import (
	"encoding/json"
	"fmt"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/nexus/core"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"
)

// MockNexusServerWithSession extends MockNexusServer to handle session requests
type MockNexusServerWithSession struct {
	*MockNexusServer
}

func NewMockNexusServerWithSession(port int) (*MockNexusServerWithSession, error) {
	base, err := NewMockNexusServer(port)
	if err != nil {
		return nil, err
	}
	return &MockNexusServerWithSession{MockNexusServer: base}, nil
}

func (s *MockNexusServerWithSession) handleConnection(conn net.Conn) {
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	for {
		var req core.WrappedRequest
		err := decoder.Decode(&req)
		if err != nil {
			return
		}

		switch req.Type {
		case core.RequestTypeSession:
			// Handle session request
			var sessionReq core.SessionRequest
			if err := json.Unmarshal(req.Data, &sessionReq); err != nil {
				continue
			}

			// Generate mock session response based on action
			var sessionResp *core.SessionResponse

			switch sessionReq.Action {
			case core.SessionActionStart:
				sessionResp = &core.SessionResponse{
					Status:    core.ResponseStatusSuccess,
					SessionID: fmt.Sprintf("session-%d", time.Now().Unix()),
					Endpoint:  "http://192.168.1.100:5000",
				}
			case core.SessionActionStop:
				sessionResp = &core.SessionResponse{
					Status:    core.ResponseStatusSuccess,
					SessionID: sessionReq.SessionID,
				}
			case core.SessionActionExtend:
				sessionResp = &core.SessionResponse{
					Status:    core.ResponseStatusSuccess,
					SessionID: sessionReq.SessionID,
					Endpoint:  "http://192.168.1.100:5000",
				}
			}

			// Send session response
			wrapped, err := core.NewWrappedResponse(req.RequestID, sessionResp)
			if err != nil {
				continue
			}
			if err := encoder.Encode(wrapped); err != nil {
				return
			}

		case core.RequestTypeInference:
			// Handle inference request
			var inferReq core.InferenceRequest
			if err := json.Unmarshal(req.Data, &inferReq); err != nil {
				continue
			}

			// Generate streaming responses
			partial := &core.InferenceResponse{
				Type:    core.ResponseTypePartial,
				Content: fmt.Sprintf("Partial response for request %s", req.RequestID),
				Status:  core.ResponseStatusSuccess,
			}

			final := &core.InferenceResponse{
				Type:    core.ResponseTypeFinal,
				Content: fmt.Sprintf("Final response for request %s", req.RequestID),
				Status:  core.ResponseStatusSuccess,
			}

			// Send partial response
			partialWrapped, err := core.NewWrappedResponse(req.RequestID, partial)
			if err != nil {
				continue
			}
			if err := encoder.Encode(partialWrapped); err != nil {
				return
			}

			// Small delay to simulate processing
			time.Sleep(50 * time.Millisecond)

			// Send final response
			finalWrapped, err := core.NewWrappedResponse(req.RequestID, final)
			if err != nil {
				continue
			}
			if err := encoder.Encode(finalWrapped); err != nil {
				return
			}
		}
	}
}

func TestSessionManagement(t *testing.T) {
	// Create a custom test server with session support
	tempDir, err := os.MkdirTemp("", "deltamind-session-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Start the mock Nexus server with session support
	mockNexus, err := NewMockNexusServerWithSession(0)
	require.NoError(t, err)
	defer mockNexus.Close()

	// Get the assigned port from the listener's address
	nexusPort := mockNexus.listener.Addr().(*net.TCPAddr).Port

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
		Server:          server,
		URL:             httpServer.URL,
		WsURL:           wsURL.String(),
		MockNexusServer: mockNexus.MockNexusServer,
		httpServer:      httpServer,
	}

	// Wait for the NexusClient to establish a connection
	require.NoError(t, ts.waitForNexusConnection(5*time.Second))

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

		// Test STOP session
		stopReq := SessionRequest{
			Action:    SessionActionStop,
			SessionID: sessionID,
			AuthToken: user.AuthToken,
		}

		stopResp, err := performRequest[SessionRequest, SessionResponse](t, ts, "POST", "/api/v0/manage-session", stopReq)
		require.NoError(t, err)
		require.Equal(t, "SUCCESS", stopResp.Status)
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

		// Start session
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

		// Now send inference request
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

		// Read inference response
		inferResp, err := receiveTypedWSMessage[WSInferenceResponse](ws)
		require.NoError(t, err)
		require.Equal(t, string(core.ResponseStatusSuccess), inferResp.Status)
		require.NotEmpty(t, inferResp.Content)

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
	})
}

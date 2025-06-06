package api

import (
	"encoding/json"
	"fmt"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/nlpfollower/deltamind/nexus/core"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
	"sync"
	"testing"
)

func sendTypedWSMessage(ws *websocket.Conn, payload WSMessagePayload) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	msg := WSMessage{
		Type:    payload.GetWSMessageType(),
		Payload: payloadBytes,
	}

	return websocket.JSON.Send(ws, msg)
}

// Helper function to receive typed websocket messages and unmarshal payload
func receiveTypedWSMessage[T WSMessagePayload](ws *websocket.Conn) (T, error) {
	var msg WSMessage
	if err := websocket.JSON.Receive(ws, &msg); err != nil {
		var zero T
		return zero, err
	}

	var payload T
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		var zero T
		return zero, err
	}

	return payload, nil
}

func TestWebSocketHandler(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	t.Run("Handshake and single inference request", func(t *testing.T) {
		user, err := createTestUser(t, ts, "ws1@example.com", "wsuser1", "password123")
		require.NoError(t, err)

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

		// Send handshake
		handshakeReq := HandshakeRequest{
			AuthToken: user.AuthToken,
		}
		err = sendTypedWSMessage(ws, handshakeReq)
		require.NoError(t, err)

		// Read handshake response
		handshakeResp, err := receiveTypedWSMessage[HandshakeResponse](ws)
		require.NoError(t, err)
		require.Equal(t, WSMessageTypeHandshake, int(handshakeResp.GetWSMessageType()))
		require.Equal(t, "success", handshakeResp.Status)

		// Send inference request
		inferReq := WSInferenceRequest{
			LastMessageID: &storage.CompoundMessageID{
				ID:        lastMessage.ID,
				MessageID: 0,
			},
			ModelID:   "gpt-4",
			AuthToken: user.AuthToken,
		}
		err = sendTypedWSMessage(ws, inferReq)
		require.NoError(t, err)

		// Read inference response
		inferResp, err := receiveTypedWSMessage[WSInferenceResponse](ws)
		require.NoError(t, err)
		require.Equal(t, WSMessageTypeInference, int(inferResp.GetWSMessageType()))
		require.Equal(t, string(core.ResponseStatusSuccess), inferResp.Status)
		require.NotEmpty(t, inferResp.Content)
	})

	t.Run("Multiple clients streaming simultaneously", func(t *testing.T) {
		numClients := 5
		origin := "http://localhost/"
		url := fmt.Sprintf("ws://%s/ws", ts.URL[7:])

		var wg sync.WaitGroup
		wg.Add(numClients)

		for i := 0; i < numClients; i++ {
			go func(clientID int) {
				defer wg.Done()

				user, err := createTestUser(t, ts, fmt.Sprintf("ws%d_concurrent@example.com", clientID), fmt.Sprintf("wsuser%d", clientID), "password123")
				require.NoError(t, err)

				space := createTestSpace(t, ts, user)
				thread := createTestThread(t, ts, user, space, fmt.Sprintf("Test Thread %d", clientID))
				messages := createTestMessages(t, ts, user, thread, 1)
				require.NotEmpty(t, messages)
				lastMessage := messages[0]

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

				// Send inference request
				inferReq := WSInferenceRequest{
					LastMessageID: &storage.CompoundMessageID{
						ID:        lastMessage.ID,
						MessageID: 0,
					},
					ModelID:   "gpt-4",
					AuthToken: user.AuthToken,
				}
				err = sendTypedWSMessage(ws, inferReq)
				require.NoError(t, err)

				// Read inference response
				inferResp, err := receiveTypedWSMessage[WSInferenceResponse](ws)
				require.NoError(t, err)
				require.Equal(t, string(core.ResponseStatusSuccess), inferResp.Status)
				require.NotEmpty(t, inferResp.Content)
				require.Contains(t, inferResp.Content, "req")
			}(i)
		}

		wg.Wait()
	})

	t.Run("Single client streaming multiple times", func(t *testing.T) {
		user, err := createTestUser(t, ts, "ws_multi@example.com", "wsuser_multi", "password123")
		require.NoError(t, err)

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

		// Send multiple inference requests
		numRequests := 3
		for i := 0; i < numRequests; i++ {
			inferReq := WSInferenceRequest{
				LastMessageID: &storage.CompoundMessageID{
					ID:        lastMessage.ID,
					MessageID: 0,
				},
				ModelID:   "gpt-4",
				AuthToken: user.AuthToken,
			}
			err = sendTypedWSMessage(ws, inferReq)
			require.NoError(t, err)

			// Read inference response
			inferResp, err := receiveTypedWSMessage[WSInferenceResponse](ws)
			require.NoError(t, err)
			require.Equal(t, string(core.ResponseStatusSuccess), inferResp.Status)
			require.NotEmpty(t, inferResp.Content)
		}
	})

	t.Run("Invalid handshake", func(t *testing.T) {
		origin := "http://localhost/"
		url := fmt.Sprintf("ws://%s/ws", ts.URL[7:])
		ws, err := websocket.Dial(url, "", origin)
		require.NoError(t, err)
		defer ws.Close()

		// Send invalid handshake
		handshakeReq := HandshakeRequest{
			AuthToken: AuthToken{SessionKey: "invalid_session_key"},
		}
		err = sendTypedWSMessage(ws, handshakeReq)
		require.NoError(t, err)

		// Read handshake response
		handshakeResp, err := receiveTypedWSMessage[HandshakeResponse](ws)
		require.NoError(t, err)
		require.Equal(t, WSMessageTypeHandshake, int(handshakeResp.GetWSMessageType()))
		require.Equal(t, "error", handshakeResp.Status)
	})

	t.Run("Invalid message ID", func(t *testing.T) {
		user, err := createTestUser(t, ts, "ws_invalid_msg@example.com", "wsuser_invalid", "password123")
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

		handshakeResp, err := receiveTypedWSMessage[HandshakeResponse](ws)
		require.NoError(t, err)
		require.Equal(t, "success", handshakeResp.Status)

		// Send request with invalid message ID
		invalidMsgID, err := db.DigestFromString("0123456789abcdef0123456789abcdef")
		require.Error(t, err)

		inferReq := WSInferenceRequest{
			LastMessageID: &storage.CompoundMessageID{
				ID:        invalidMsgID,
				MessageID: 0,
			},
			ModelID:   "gpt-4",
			AuthToken: user.AuthToken,
		}
		err = sendTypedWSMessage(ws, inferReq)
		require.NoError(t, err)

		inferResp, err := receiveTypedWSMessage[WSInferenceResponse](ws)
		require.NoError(t, err)
		require.Equal(t, "error", inferResp.Status)
	})
}

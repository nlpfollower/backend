package api

import (
	"encoding/json"
	"fmt"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/nlpfollower/deltamind/nexus/core"
	"github.com/nlpfollower/deltamind/orchestration/utils"
	"golang.org/x/net/websocket"
	"io"
	"log"
	"time"
)

const MaxHistoryMessages = 10

// WebSocket message types
type WSMessageType int

const (
	WSMessageTypeHandshake = 1
	WSMessageTypeInference = 2
)

type WSMessage struct {
	Type    WSMessageType   `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type WSMessagePayload interface {
	GetWSMessageType() WSMessageType
}

type WebSocketHandler struct {
	nexusClient     *NexusClient
	dbManager       *storage.DatabaseManager
	clients         *utils.ConcurrentMap[string, *websocket.Conn]
	pendingMessages *utils.ConcurrentMap[string, *MessageBuilder]
}

type MessageBuilder struct {
	ThreadID    db.Digest
	UserMessage *storage.CompoundMessage
	Content     string
}

type HandshakeRequest struct {
	AuthToken AuthToken `json:"auth_token"`
}

func (h HandshakeRequest) GetWSMessageType() WSMessageType {
	return WSMessageTypeHandshake
}

type HandshakeResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (h HandshakeResponse) GetWSMessageType() WSMessageType {
	return WSMessageTypeHandshake
}

// WebSocket message types
type WSInferenceRequest struct {
	LastMessageID *storage.CompoundMessageID `json:"last_message_id"` // ID of the last message in thread
	ModelID       string                     `json:"model_id"`        // ID of the model to use for inference
	AuthToken     AuthToken                  `json:"auth_token"`
}

func (i WSInferenceRequest) GetWSMessageType() WSMessageType {
	return WSMessageTypeInference
}

type WSInferenceResponse struct {
	Content string `json:"content"`
	Type    string `json:"type"` // "partial" or "final"
	Status  string `json:"status"`
}

func (i WSInferenceResponse) GetWSMessageType() WSMessageType {
	return WSMessageTypeInference
}

func NewWebSocketHandler(nexusClient *NexusClient, dbManager *storage.DatabaseManager) *WebSocketHandler {
	return &WebSocketHandler{
		nexusClient:     nexusClient,
		dbManager:       dbManager,
		clients:         utils.NewConcurrentMap[string, *websocket.Conn](),
		pendingMessages: utils.NewConcurrentMap[string, *MessageBuilder](),
	}
}

func (wsh *WebSocketHandler) HandleWebSocket(ws *websocket.Conn) {
	var userID string
	defer func() {
		if userID != "" {
			wsh.clients.Remove(userID)
		}
	}()

	// Handle incoming messages
	for {
		var msg WSMessage
		if err := websocket.JSON.Receive(ws, &msg); err != nil {
			if err != io.EOF {
				log.Printf("Error reading message: %v", err)
			}
			break
		}

		switch msg.Type {
		case WSMessageTypeHandshake:
			var handshakeReq HandshakeRequest
			if err := json.Unmarshal(msg.Payload, &handshakeReq); err != nil {
				resp := HandshakeResponse{
					Status:  "error",
					Message: "Invalid handshake format",
				}
				sendTypedWSResponse(ws, resp)
				continue
			}

			claims, err := ValidateSessionKey(handshakeReq.AuthToken.SessionKey)
			if err != nil {
				log.Printf("Invalid handshake: %v", err)
				resp := HandshakeResponse{
					Status:  "error",
					Message: err.Error(),
				}
				sendTypedWSResponse(ws, resp)
				continue
			}

			userID = claims.UserID
			wsh.clients.Set(userID, ws)

			resp := HandshakeResponse{
				Status:  "success",
				Message: "Handshake successful",
			}
			sendTypedWSResponse(ws, resp)

		case WSMessageTypeInference:
			if userID == "" {
				resp := WSInferenceResponse{
					Status:  "error",
					Content: "Handshake required before inference",
					Type:    "error",
				}
				sendTypedWSResponse(ws, resp)
				continue
			}

			var inferReq WSInferenceRequest
			if err := json.Unmarshal(msg.Payload, &inferReq); err != nil {
				resp := WSInferenceResponse{
					Status:  "error",
					Content: "Invalid inference request format",
					Type:    "error",
				}
				sendTypedWSResponse(ws, resp)
				continue
			}

			// Validate the request's auth token
			reqClaims, err := ValidateSessionKey(inferReq.AuthToken.SessionKey)
			if err != nil || reqClaims.UserID != userID {
				resp := WSInferenceResponse{
					Status:  "error",
					Content: "Invalid or mismatched auth token",
					Type:    "error",
				}
				sendTypedWSResponse(ws, resp)
				continue
			}

			// Generate request ID internally
			requestIDBytes, err := GenerateRandomBytes(32)
			if err != nil {
				resp := WSInferenceResponse{
					Status:  "error",
					Content: "Failed to generate request ID",
					Type:    "error",
				}
				sendTypedWSResponse(ws, resp)
				continue
			}
			requestID := db.NewDigest(requestIDBytes)

			// Handle inference in a separate goroutine
			go wsh.handleInferenceRequest(ws, userID, requestID, inferReq)

		default:
			// Ignore unknown message types
			log.Printf("Received unknown message type: %d", msg.Type)
		}
	}
}

func sendTypedWSResponse(ws *websocket.Conn, payload WSMessagePayload) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling response: %v", err)
		return
	}

	response := WSMessage{
		Type:    payload.GetWSMessageType(),
		Payload: payloadBytes,
	}

	if err := websocket.JSON.Send(ws, response); err != nil {
		log.Printf("Error sending response: %v", err)
	}
}

func (wsh *WebSocketHandler) validateHandshake(handshakeReq *HandshakeRequest) error {
	// Validate the AuthToken
	claims, err := ValidateSessionKey(handshakeReq.AuthToken.SessionKey)
	if err != nil {
		return fmt.Errorf("invalid auth token: %v", err)
	}

	// Verify the user exists
	userID, err := db.DigestFromString(claims.UserID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %v", err)
	}

	if err := wsh.dbManager.View(func(txn *storage.DatabaseTransaction) error {
		user, err := txn.GetUser(userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %v", err)
		}
		// Check if user exists
		if user == nil {
			return fmt.Errorf("user not found")
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (wsh *WebSocketHandler) handleInferenceRequest(ws *websocket.Conn, userID string, requestID db.Digest, req WSInferenceRequest) {
	// Get user ID as digest
	userIDDigest, err := db.DigestFromString(userID)
	if err != nil {
		resp := WSInferenceResponse{
			Status:  "error",
			Content: "Invalid user ID",
			Type:    "error",
		}
		sendTypedWSResponse(ws, resp)
		return
	}

	var messages []*storage.CompoundMessage
	var checkpointPath string // NEW: Track checkpoint path

	// Verify access and get message chain in a single transaction
	err = wsh.dbManager.View(func(txn *storage.DatabaseTransaction) error {
		log.Printf("DEBUG: Starting transaction for inference request")
		log.Printf("DEBUG: LastMessageID.ID: %v", req.LastMessageID.ID)
		log.Printf("DEBUG: LastMessageID.MessageID: %v", req.LastMessageID.MessageID)
		log.Printf("DEBUG: ModelID: %s", req.ModelID)

		// First verify the user has access to this message
		lastMessage, err := txn.GetMessage(req.LastMessageID.ID)
		if err != nil {
			log.Printf("DEBUG: Error getting message: %v", err)
			return fmt.Errorf("failed to get message: %v", err)
		}
		if lastMessage == nil {
			log.Printf("DEBUG: Message not found for ID: %v", req.LastMessageID.ID)
			return fmt.Errorf("message not found")
		}
		log.Printf("DEBUG: Found message, ThreadID: %v", lastMessage.ThreadID)

		// Get the parent thread
		thread, err := txn.GetThread(lastMessage.ThreadID)
		if err != nil {
			log.Printf("DEBUG: Error getting thread: %v", err)
			return fmt.Errorf("failed to get thread: %v", err)
		}
		if thread == nil {
			log.Printf("DEBUG: Thread is nil for ID: %v", lastMessage.ThreadID)
			return fmt.Errorf("thread not found")
		}
		log.Printf("DEBUG: Found thread, SpaceID: %v", thread.SpaceID)

		// Get the parent space
		space, err := txn.GetSpace(thread.SpaceID)
		if err != nil {
			log.Printf("DEBUG: Error getting space: %v", err)
			return fmt.Errorf("failed to get space: %v", err)
		}
		if space == nil {
			log.Printf("DEBUG: Space is nil for ID: %v", thread.SpaceID)
			return fmt.Errorf("space not found")
		}
		log.Printf("DEBUG: Found space, UserID: %v, checking against: %v", space.UserID, userIDDigest)

		// Verify user has access
		if space.UserID != userIDDigest {
			log.Printf("DEBUG: User access denied - space.UserID: %v, userIDDigest: %v", space.UserID, userIDDigest)
			return fmt.Errorf("user does not have access to this thread")
		}
		log.Printf("DEBUG: User has access, proceeding to get model")

		// Get model info to retrieve checkpoint path
		var model *storage.ModelInfo

		// First try direct lookup
		modelDigest := db.NewDigest([]byte(req.ModelID))
		log.Printf("DEBUG: Looking up model with digest: %v (from ModelID: %s)", modelDigest, req.ModelID)

		model, err = txn.GetModel(modelDigest)
		log.Printf("DEBUG: Direct model lookup result - model: %v, err: %v", model, err)

		if err != nil || model == nil {
			log.Printf("DEBUG: Direct lookup failed, trying fallback methods")

			// Try to get by name if digest lookup fails
			models, err := txn.GetUserModels(userIDDigest, 100, uint64(time.Now().UnixNano()))
			if err != nil {
				log.Printf("DEBUG: Failed to get user models: %v", err)
				return fmt.Errorf("failed to get user models: %v", err)
			}
			log.Printf("DEBUG: Found %d user models", len(models))

			// Also check base models
			systemUserID := db.NewDigest([]byte("system-base-models"))
			log.Printf("DEBUG: Looking up base models with systemUserID: %v", systemUserID)

			baseModels, err := txn.GetUserModels(systemUserID, 10, uint64(time.Now().UnixNano()))
			if err == nil {
				log.Printf("DEBUG: Found %d base models", len(baseModels))
				models = append(models, baseModels...)
			} else {
				log.Printf("DEBUG: Error getting base models: %v", err)
			}

			// Find model by name
			log.Printf("DEBUG: Searching through %d total models for name: %s", len(models), req.ModelID)
			for i, m := range models {
				log.Printf("DEBUG: Model[%d] - Name: %s, ID: %v", i, m.Name, m.ID)
				if m.Name == req.ModelID {
					model = m
					log.Printf("DEBUG: Found matching model by name!")
					break
				}
			}

			if model == nil {
				log.Printf("DEBUG: Model still nil after all lookups")
				return fmt.Errorf("model not found: %s", req.ModelID)
			}
		}

		log.Printf("DEBUG: Model found - Name: %s, CheckpointPath: %s", model.Name, model.CheckpointPath)

		// Resolve checkpoint path
		if model.CheckpointPath != "" {
			checkpointPath = model.CheckpointPath
			log.Printf("DEBUG: Using model's checkpoint path: %s", checkpointPath)
		} else {
			// Fallback for models without explicit checkpoint path
			checkpointPath = fmt.Sprintf("/mnt/cold/contents/dcp/%s/checkpoint", model.Name)
			log.Printf("DEBUG: Using fallback checkpoint path: %s", checkpointPath)
		}

		// Now get the message chain
		log.Printf("DEBUG: Getting message path for ID: %v, MessageID: %d", req.LastMessageID.ID, req.LastMessageID.MessageID)
		messages, err = txn.GetMessagePath(req.LastMessageID.ID, req.LastMessageID.MessageID, MaxHistoryMessages)
		if err != nil {
			log.Printf("DEBUG: Error getting message path: %v", err)
		} else {
			log.Printf("DEBUG: Got %d messages in path", len(messages))
		}

		return err
	})

	if err != nil {
		resp := WSInferenceResponse{
			Status:  "error",
			Content: err.Error(),
			Type:    "error",
		}
		sendTypedWSResponse(ws, resp)
		return
	}

	// Convert messages to inference format
	contextMessages := make([]core.Message, 0, len(messages))
	for _, msg := range messages {
		var content string
		if msg.ID == req.LastMessageID.ID {
			content = msg.Messages[req.LastMessageID.MessageID]
		} else {
			// Find which message points to this one as parent
			for _, child := range messages {
				if child.ParentID != nil && child.ParentID.ID == msg.ID {
					content = msg.Messages[child.ParentID.MessageID]
					break
				}
			}
		}

		contextMessages = append(contextMessages, core.Message{
			Role:    msg.Author,
			Content: content,
		})
	}

	// Start inference with checkpoint path
	responseChan, err := wsh.nexusClient.EnqueueInference(
		userIDDigest,
		req.ModelID,
		contextMessages,
		checkpointPath,
	)
	if err != nil {
		resp := WSInferenceResponse{
			Status:  "error",
			Content: fmt.Sprintf("Failed to enqueue inference: %v", err),
			Type:    "error",
		}
		sendTypedWSResponse(ws, resp)
		return
	}

	// Add a "loading" response to indicate model is being loaded
	loadingResp := WSInferenceResponse{
		Status:  "loading",
		Content: "Loading model...",
		Type:    "partial",
	}
	sendTypedWSResponse(ws, loadingResp)

	// Stream responses
	for response := range responseChan {
		var inferResp core.InferenceResponse
		if err := json.Unmarshal(response.Data, &inferResp); err != nil {
			resp := WSInferenceResponse{
				Status:  "error",
				Content: err.Error(),
				Type:    "error",
			}
			sendTypedWSResponse(ws, resp)
			return
		}

		wsResp := WSInferenceResponse{
			Content: inferResp.Content,
			Type:    string(inferResp.Type),
			Status:  string(inferResp.Status),
		}

		sendTypedWSResponse(ws, wsResp)
	}
}

func (wsh *WebSocketHandler) sendResponseToClient(userID string, resp WSInferenceResponse) {
	if conn, ok := wsh.clients.Get(userID); ok {
		err := websocket.JSON.Send(conn, resp)
		if err != nil {
			log.Printf("Error sending response to client %s: %v", userID, err)
		}
	} else {
		log.Printf("Client %s not found", userID)
	}
}

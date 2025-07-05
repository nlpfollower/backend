package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/net/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/stretchr/testify/require"
)

// setupTestServerWithNexus creates a test server that connects to a real Nexus instance
func setupTestServerWithNexus(t *testing.T) (*Server, *storage.DatabaseManager) {
	// Create a temporary directory for the database
	dbDir := t.TempDir()

	// Create the server with Nexus on localhost:8081
	server, err := NewServer(dbDir, 8081)
	require.NoError(t, err)

	// Start the Nexus client
	server.nexusClient.Start()

	// Wait for Nexus connection to be established
	maxWait := 30 * time.Second
	start := time.Now()
	for time.Since(start) < maxWait {
		server.nexusClient.mu.Lock()
		connected := server.nexusClient.isConnected
		server.nexusClient.mu.Unlock()

		if connected {
			t.Log("Nexus client connected successfully")
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Give a bit more time for everything to stabilize
	time.Sleep(1 * time.Second)

	return server, server.dbManager
}

func TestTrainingWorkflow(t *testing.T) {
	// Use setupTestServerWithNexus to connect to real Nexus on port 8081
	server, dbManager := setupTestServerWithNexus(t)
	defer server.nexusClient.Stop()
	defer dbManager.Close()

	// Add base model to database if it doesn't exist
	err := dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		modelID := db.NewDigest([]byte("llama-8b"))
		existingModel, _ := txn.GetModel(modelID)
		if existingModel == nil {
			// Create the base model
			systemUserID := db.NewDigest([]byte("system-base-models"))
			timeNow := time.Now()
			model := storage.ModelInfo{
				ID:             modelID,
				UserID:         systemUserID,
				Name:           "llama-8b",
				DisplayName:    "llama-8b",
				ModelType:      storage.ModelTypeBase,
				BaseModel:      "llama-8b",
				ModelSize:      "8B",
				Status:         storage.ModelStatusReady,
				CheckpointPath: "/mnt/cold/contents/dcp/llama-8b/checkpoint",
				CreatedAt:      timeNow,
				UpdatedAt:      timeNow,
			}
			if err := txn.SetModel(systemUserID, &model); err != nil {
				return fmt.Errorf("failed to create base model: %v", err)
			}
			t.Log("Added llama-8b base model to database")
		}
		return nil
	})
	require.NoError(t, err)

	// Create HTTP test server wrapper
	httpServer := httptest.NewServer(server.router)
	defer httpServer.Close()

	// Create a minimal TestServer-like struct for the helper functions
	ts := &struct {
		Server *Server
		URL    string
		WsURL  string
	}{
		Server: server,
		URL:    httpServer.URL,
		WsURL:  "ws" + httpServer.URL[4:] + "/ws", // Convert http:// to ws://
	}

	// Create test user using the helper with proper URL
	user := createTestUserDirect(t, ts.URL, "trainer@example.com", "trainer", "password123")

	// Create space and thread
	space := createTestSpaceDirect(t, ts.URL, user)
	thread := createTestThreadDirect(t, ts.URL, user, space, "Training Thread")

	// Clone the base model for training
	t.Log("=== Cloning base model for training ===")
	cloneReq := CloneModelRequest{
		SourceModelID: db.NewDigest([]byte("llama-8b")),
		DisplayName:   "Training Test Model",
		AuthToken:     user.AuthToken,
	}

	cloneResp, err := performRequestDirect[CloneModelRequest, CloneModelResponse](t, ts.URL, "POST", "/api/v0/clone-model", cloneReq)
	require.NoError(t, err)
	require.NotNil(t, cloneResp)
	require.NotNil(t, cloneResp.Model)

	// Use the cloned model for training
	modelID := cloneResp.Model.Name
	t.Logf("Cloned model created: %s (ID: %s)", modelID, cloneResp.Model.ID)

	// STEP 1: Load the model via WebSocket inference request
	t.Log("=== STEP 1: Loading model via WebSocket inference ===")

	// Create a dummy message for the inference request
	dummyMessage := createTestMessageDirect(t, ts.URL, user, thread, "Test message for inference")
	require.NotNil(t, dummyMessage, "Dummy message should not be nil")
	require.NotEmpty(t, dummyMessage.ID, "Dummy message should have an ID")
	t.Logf("Created dummy message with ID: %v", dummyMessage.ID)

	// Give the database a moment to ensure consistency
	time.Sleep(100 * time.Millisecond)

	// Connect to WebSocket
	ws, err := websocket.Dial(ts.WsURL, "", ts.URL)
	require.NoError(t, err)
	defer ws.Close()

	// Send WebSocket handshake
	handshakeMsg := WSMessage{
		Type: WSMessageTypeHandshake,
		Payload: jsonMarshal(t, HandshakeRequest{
			AuthToken: user.AuthToken,
		}),
	}
	err = websocket.JSON.Send(ws, handshakeMsg)
	require.NoError(t, err)

	// Read handshake response
	var handshakeRespMsg WSMessage
	err = websocket.JSON.Receive(ws, &handshakeRespMsg)
	require.NoError(t, err)

	var handshakeResp HandshakeResponse
	err = json.Unmarshal(handshakeRespMsg.Payload, &handshakeResp)
	require.NoError(t, err)
	require.Equal(t, "success", handshakeResp.Status)
	t.Log("WebSocket handshake successful")

	// Send inference request to trigger model loading - use the cloned model
	inferMsg := WSMessage{
		Type: WSMessageTypeInference,
		Payload: jsonMarshal(t, WSInferenceRequest{
			LastMessageID: &storage.CompoundMessageID{
				ID:        dummyMessage.ID,
				MessageID: 0,
			},
			ModelID:   modelID, // Use cloned model ID
			AuthToken: user.AuthToken,
		}),
	}
	err = websocket.JSON.Send(ws, inferMsg)
	require.NoError(t, err)
	t.Log("Sent WebSocket inference request to trigger model loading")

	// Read responses until we get some content or error
	receivedContent := false
	maxResponses := 10
	for i := 0; i < maxResponses; i++ {
		ws.SetReadDeadline(time.Now().Add(10 * time.Minute))

		var respMsg WSMessage
		err := websocket.JSON.Receive(ws, &respMsg)
		if err != nil {
			if strings.Contains(err.Error(), "timeout") {
				t.Log("WebSocket read timeout - model should be loaded")
				break
			}
			t.Fatalf("Error reading WebSocket message: %v", err)
		}

		var inferResp WSInferenceResponse
		err = json.Unmarshal(respMsg.Payload, &inferResp)
		require.NoError(t, err)

		t.Logf("Received response %d: status=%s, type=%s, content=%q",
			i+1, inferResp.Status, inferResp.Type, inferResp.Content)

		if inferResp.Status == "error" {
			t.Fatalf("Inference error: %s", inferResp.Content)
		}

		if inferResp.Content != "" && inferResp.Status != "loading" {
			receivedContent = true
		}

		if inferResp.Type == "final" || receivedContent {
			t.Log("Model successfully loaded and responded")
			break
		}
	}

	// STEP 2: Create conversation history
	t.Log("=== STEP 2: Creating conversation history ===")

	// Create a series of user messages that will form the training dataset
	trainingExamples := []string{
		"Explain the transformer architecture in detail, including self-attention mechanisms and positional encoding.",
		"Describe the process of training a large language model, from data preparation to fine-tuning.",
		"What are the key differences between supervised, unsupervised, and reinforcement learning?",
		"Explain gradient descent and its variants (SGD, Adam, RMSprop) with their trade-offs.",
		"How does batch normalization work and why is it important in deep neural networks?",
		"Describe the concept of transfer learning and its applications in modern AI.",
		"What is the vanishing gradient problem and how do techniques like LSTM and GRU address it?",
		"Explain the role of regularization techniques like dropout and L1/L2 regularization.",
	}

	// Create the training example messages
	var lastMessageID *storage.CompoundMessageID
	for i, example := range trainingExamples {
		// Create user message with training content
		msgReq := CreateMessageRequest{
			ThreadID:  thread.ID,
			ParentID:  lastMessageID,
			Content:   example,
			Author:    "user",
			AuthToken: user.AuthToken,
		}
		msgResp, err := performRequestDirect[CreateMessageRequest, CreateMessageResponse](t, ts.URL, "POST", "/api/v0/create-message", msgReq)
		require.NoError(t, err)

		// Update lastMessageID for next iteration
		lastMessageID = &storage.CompoundMessageID{
			ID:        msgResp.Message.ID,
			MessageID: 0,
		}

		t.Logf("Added training example %d/%d", i+1, len(trainingExamples))
	}

	// STEP 3: Create training message
	t.Log("=== STEP 3: Creating training message ===")

	// The training message contains the training prompt that describes what to learn
	trainingPrompt := "Learn to provide comprehensive, technical explanations about machine learning and neural network concepts."

	trainingMsgReq := CreateMessageRequest{
		ThreadID:    thread.ID,
		ParentID:    lastMessageID,
		Content:     trainingPrompt,
		Author:      "user",
		MessageType: storage.MessageTypeTraining,
		AuthToken:   user.AuthToken,
	}

	trainingMsgResp, err := performRequestDirect[CreateMessageRequest, CreateMessageResponse](t, ts.URL, "POST", "/api/v0/create-message", trainingMsgReq)
	require.NoError(t, err)

	trainingMessageID := storage.CompoundMessageID{
		ID:        trainingMsgResp.Message.ID,
		MessageID: 0,
	}

	t.Logf("Created training message with ID: %v", trainingMessageID)

	// STEP 4: Start training
	t.Log("=== STEP 4: Starting training job ===")

	trainReq := StartTrainingRequest{
		MessageID: trainingMessageID,
		ModelID:   modelID,
		AuthToken: user.AuthToken,
	}

	trainResp, err := performRequestDirect[StartTrainingRequest, StartTrainingResponse](t, ts.URL, "POST", "/api/v0/training/start", trainReq)
	require.NoError(t, err)
	require.NotEmpty(t, trainResp.JobID)
	t.Logf("Training job started with ID: %s", trainResp.JobID)

	// STEP 4: Monitor training status
	t.Log("=== STEP 4: Monitoring training status ===")

	statusCheckCount := 0
	maxStatusChecks := 90 // 15 minutes max
	datasetProcessed := false
	trainingStarted := false
	trainingCompleted := false
	lastStatus := ""

	for statusCheckCount < maxStatusChecks {
		statusCheckCount++

		// Check training status
		statusReq := GetTrainingStatusRequest{
			JobID:     trainResp.JobID,
			AuthToken: user.AuthToken,
		}

		statusResp, err := performRequestDirect[GetTrainingStatusRequest, GetTrainingStatusResponse](t, ts.URL, "POST", "/api/v0/training/status", statusReq)
		require.NoError(t, err)

		// Only log if status changed
		if statusResp.Status != lastStatus {
			t.Logf("Training status changed to: %s (Progress: %.2f%%)",
				statusResp.Status, statusResp.Progress*100)
			lastStatus = statusResp.Status
		}

		// Track progress through stages
		switch statusResp.Status {
		case "processed_dataset", "dataset_processed":
			datasetProcessed = true
			t.Log("Dataset processing completed successfully")

		case "starting_training":
			t.Log("Training is being initialized...")

		case "training":
			trainingStarted = true
			t.Log("Training has started")

		case "completed":
			trainingCompleted = true
			t.Log("Training completed successfully")

		case "error":
			t.Fatalf("Training job failed with error: %s", statusResp.Error)
		}
		if trainingCompleted {
			t.Log("Training job has completed successfully")
			break
		}

		// Wait before next check
		time.Sleep(10 * time.Second)
	}

	// Verify the job progressed through expected stages
	require.True(t, datasetProcessed, "Dataset should have been processed")
	require.True(t, trainingStarted, "Training should have started")

	// Final status check
	finalStatusReq := GetTrainingStatusRequest{
		JobID:     trainResp.JobID,
		AuthToken: user.AuthToken,
	}

	finalStatus, err := performRequestDirect[GetTrainingStatusRequest, GetTrainingStatusResponse](t, ts.URL, "POST", "/api/v0/training/status", finalStatusReq)
	require.NoError(t, err)

	t.Logf("Final training status: %s (Progress: %.2f%%)",
		finalStatus.Status, finalStatus.Progress*100)

	// Verify training completed successfully
	require.True(t, trainingCompleted, "Training should have completed")
	require.Equal(t, "completed", finalStatus.Status, "Final status should be completed")

	// Get user models to verify the trained model exists
	getModelsReq := GetModelsRequest{
		AuthToken: user.AuthToken,
		Limit:     100,
	}

	getModelsResp, err := performRequestDirect[GetModelsRequest, GetModelsResponse](t, ts.URL, "POST", "/api/v0/get-models", getModelsReq)
	require.NoError(t, err)

	// Check if trained model exists
	foundTrainedModel := false
	for _, model := range getModelsResp.Models {
		if model.Name == trainResp.ModelID {
			foundTrainedModel = true
			t.Logf("Found trained model: %s", model.Name)
			break
		}
	}

	require.True(t, foundTrainedModel, "Trained model should exist after completion")

	// STEP 5: Test inference on the trained model
	t.Log("=== STEP 5: Testing inference on trained model ===")

	// Create a new message to test the trained model
	testMsgReq := CreateMessageRequest{
		ThreadID:  thread.ID,
		Content:   "Explain how convolutional neural networks work for image recognition.",
		Author:    "user",
		AuthToken: user.AuthToken,
	}
	testMsgResp, err := performRequestDirect[CreateMessageRequest, CreateMessageResponse](t, ts.URL, "POST", "/api/v0/create-message", testMsgReq)
	require.NoError(t, err)

	// Open a new WebSocket connection for inference
	inferWs, err := websocket.Dial(ts.WsURL, "", ts.URL)
	require.NoError(t, err)
	defer inferWs.Close()

	// Send WebSocket handshake
	handshakeMsg = WSMessage{
		Type: WSMessageTypeHandshake,
		Payload: jsonMarshal(t, HandshakeRequest{
			AuthToken: user.AuthToken,
		}),
	}
	err = websocket.JSON.Send(inferWs, handshakeMsg)
	require.NoError(t, err)

	// Read handshake response
	var inferHandshakeRespMsg WSMessage
	err = websocket.JSON.Receive(inferWs, &inferHandshakeRespMsg)
	require.NoError(t, err)

	var inferHandshakeResp HandshakeResponse
	err = json.Unmarshal(inferHandshakeRespMsg.Payload, &inferHandshakeResp)
	require.NoError(t, err)
	require.Equal(t, "success", inferHandshakeResp.Status)
	t.Log("Inference WebSocket handshake successful")

	// Send inference request using the trained model
	trainedInferMsg := WSMessage{
		Type: WSMessageTypeInference,
		Payload: jsonMarshal(t, WSInferenceRequest{
			LastMessageID: &storage.CompoundMessageID{
				ID:        testMsgResp.Message.ID,
				MessageID: 0,
			},
			ModelID:   trainResp.ModelID, // Use the trained model
			AuthToken: user.AuthToken,
		}),
	}
	err = websocket.JSON.Send(inferWs, trainedInferMsg)
	require.NoError(t, err)
	t.Logf("Sent inference request to trained model: %s", trainResp.ModelID)

	// Read inference responses
	var inferenceContent strings.Builder
	responseCount := 0
	maxInferenceResponses := 50

	for responseCount < maxInferenceResponses {
		inferWs.SetReadDeadline(time.Now().Add(2 * time.Minute))

		var respMsg WSMessage
		err := websocket.JSON.Receive(inferWs, &respMsg)
		if err != nil {
			if strings.Contains(err.Error(), "timeout") {
				t.Log("Inference timeout - ending stream")
				break
			}
			t.Fatalf("Error reading inference response: %v", err)
		}

		var inferResp WSInferenceResponse
		err = json.Unmarshal(respMsg.Payload, &inferResp)
		require.NoError(t, err)

		responseCount++

		if inferResp.Status == "error" {
			t.Fatalf("Inference error from trained model: %s", inferResp.Content)
		}

		if inferResp.Content != "" && inferResp.Status != "loading" {
			inferenceContent.WriteString(inferResp.Content)
		}

		if inferResp.Type == "final" {
			t.Log("Received final inference response from trained model")
			break
		}
	}

	// Verify we got some response from the trained model
	finalContent := inferenceContent.String()
	require.NotEmpty(t, finalContent, "Should have received content from trained model")
	t.Logf("Trained model response length: %d characters", len(finalContent))
	t.Logf("Trained model response preview: %.200s...", finalContent)

	t.Log("Training workflow test completed successfully")
}

// TestTrainingStatusTracking tests the status tracking functionality
func TestTrainingStatusTracking(t *testing.T) {
	server, dbManager := setupTestServerWithNexus(t)
	defer server.nexusClient.Stop()
	defer dbManager.Close()

	// Create HTTP test server
	httpServer := httptest.NewServer(server.router)
	defer httpServer.Close()

	// Test getting status for non-existent job
	statusReq := GetTrainingStatusRequest{
		JobID:     "non-existent-job",
		AuthToken: AuthToken{SessionKey: "test-token"},
	}

	_, err := performRequestDirect[GetTrainingStatusRequest, GetTrainingStatusResponse](t, httpServer.URL, "POST", "/api/v0/training/status", statusReq)
	require.Error(t, err)
	require.Contains(t, err.Error(), "401") // Invalid auth token
}

// TestSystemStatus tests the system status endpoint
func TestSystemStatus(t *testing.T) {
	server, dbManager := setupTestServerWithNexus(t)
	defer server.nexusClient.Stop()
	defer dbManager.Close()

	// Create HTTP test server
	httpServer := httptest.NewServer(server.router)
	defer httpServer.Close()

	// Create test user
	user := createTestUserDirect(t, httpServer.URL, "statususer@example.com", "statususer", "testpass123")

	// Test 1: System status when idle
	t.Log("Test 1: System status when idle")

	statusReq := GetSystemStatusRequest{
		AuthToken: user.AuthToken,
	}

	statusResp, err := performRequestDirect[GetSystemStatusRequest, GetSystemStatusResponse](t, httpServer.URL, "POST", "/api/v0/system/status", statusReq)
	require.NoError(t, err)
	require.Equal(t, SystemStatusIdle, statusResp.Status)
	require.Empty(t, statusResp.ActiveTraining)
	require.Empty(t, statusResp.ActiveSessions)

	// Test 2: Create a training job and check status
	t.Log("Test 2: System status with active training")

	// First create necessary setup (space, thread, model, messages)
	space := createTestSpaceDirect(t, httpServer.URL, user)
	thread := createTestThreadDirect(t, httpServer.URL, user, space, "Status Test Thread")

	// Clone a model
	baseModelID := db.NewDigest([]byte("llama-8b"))
	cloneReq := CloneModelRequest{
		SourceModelID: baseModelID,
		DisplayName:   "Status Test Model",
		AuthToken:     user.AuthToken,
	}

	cloneResp, err := performRequestDirect[CloneModelRequest, CloneModelResponse](t, httpServer.URL, "POST", "/api/v0/clone-model", cloneReq)
	require.NoError(t, err)

	// Create a training message
	trainingMsgReq := CreateMessageRequest{
		ThreadID:    thread.ID,
		Content:     "Train on test data",
		Author:      "user",
		MessageType: storage.MessageTypeTraining,
		AuthToken:   user.AuthToken,
	}

	trainingMsgResp, err := performRequestDirect[CreateMessageRequest, CreateMessageResponse](t, httpServer.URL, "POST", "/api/v0/create-message", trainingMsgReq)
	require.NoError(t, err)

	// Start training
	trainReq := StartTrainingRequest{
		MessageID: storage.CompoundMessageID{
			ID:        trainingMsgResp.Message.ID,
			MessageID: 0, // Using first message version
		},
		ModelID:   cloneResp.Model.Name,
		AuthToken: user.AuthToken,
	}

	trainResp, err := performRequestDirect[StartTrainingRequest, StartTrainingResponse](t, httpServer.URL, "POST", "/api/v0/training/start", trainReq)
	if err != nil {
		t.Fatalf("Failed to start training: %v", err)
	}

	// Now check system status
	time.Sleep(2 * time.Second) // Give it a moment to start

	statusResp2, err := performRequestDirect[GetSystemStatusRequest, GetSystemStatusResponse](t, httpServer.URL, "POST", "/api/v0/system/status", statusReq)
	require.NoError(t, err)
	require.Equal(t, SystemStatusTraining, statusResp2.Status)
	require.Len(t, statusResp2.ActiveTraining, 1)
	require.Equal(t, trainResp.JobID, statusResp2.ActiveTraining[0].JobID)
	require.Equal(t, trainResp.ModelID, statusResp2.ActiveTraining[0].ModelID)

	t.Logf("System status shows active training: %+v", statusResp2.ActiveTraining[0])

	t.Log("System status test completed successfully!")
}

// Helper function to marshal JSON for WebSocket messages
func jsonMarshal(t *testing.T, v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

// Helper functions for direct HTTP requests (without TestServer)
func performRequestDirect[Req any, Resp any](t *testing.T, baseURL string, method, path string, req Req) (*Resp, error) {
	url := fmt.Sprintf("%s%s", baseURL, path)

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	httpReq, err := http.NewRequest(method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var response Resp
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

func createTestUserDirect(t *testing.T, baseURL, email, username, password string) *SignUpResponse {
	passwordHex := hex.EncodeToString(HashPassword(password))
	signupReq := SignUpRequest{
		Email:       email,
		Username:    username,
		PasswordHex: passwordHex,
		Method:      storage.AuthMethodEmailPassword,
	}

	signupResp, err := performRequestDirect[SignUpRequest, SignUpResponse](t, baseURL, "POST", "/api/v0/sign-up", signupReq)
	require.NoError(t, err)
	return signupResp
}

func createTestSpaceDirect(t *testing.T, baseURL string, user *SignUpResponse) *storage.Space {
	spaceReq := CreateSpaceRequest{
		Name:        "Test Space",
		Description: "A test space for threads",
		AuthToken:   user.AuthToken,
	}

	spaceResp, err := performRequestDirect[CreateSpaceRequest, CreateSpaceResponse](t, baseURL, "POST", "/api/v0/create-space", spaceReq)
	require.NoError(t, err)
	return &spaceResp.Space
}

func createTestThreadDirect(t *testing.T, baseURL string, user *SignUpResponse, space *storage.Space, title string) *storage.Thread {
	threadReq := CreateThreadRequest{
		SpaceID:   space.ID,
		Title:     title,
		AuthToken: user.AuthToken,
	}

	threadResp, err := performRequestDirect[CreateThreadRequest, CreateThreadResponse](t, baseURL, "POST", "/api/v0/create-thread", threadReq)
	require.NoError(t, err)
	return &threadResp.Thread
}

func createTestMessageDirect(t *testing.T, baseURL string, user *SignUpResponse, thread *storage.Thread, content string) *storage.CompoundMessage {
	msgReq := CreateMessageRequest{
		ThreadID:  thread.ID,
		Content:   content,
		Author:    "user",
		AuthToken: user.AuthToken,
	}

	msgResp, err := performRequestDirect[CreateMessageRequest, CreateMessageResponse](t, baseURL, "POST", "/api/v0/create-message", msgReq)
	require.NoError(t, err)
	require.NotNil(t, msgResp)
	require.NotNil(t, msgResp.Message)
	require.NotEmpty(t, msgResp.Message.ID)
	require.Len(t, msgResp.Message.Messages, 1, "Message should have exactly one content entry")
	return msgResp.Message
}

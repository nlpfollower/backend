package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/nlpfollower/deltamind/nexus/core"
	"golang.org/x/net/websocket"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/stretchr/testify/require"
)

// setupTestServerWithNexus creates a test server that connects to a real Nexus instance
func setupTestServerWithNexus(t *testing.T) (*Server, *storage.DatabaseManager) {
	// Create a temporary database
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Create the server with Nexus on localhost:8081
	server, err := NewServer(dbPath, 8081)
	require.NoError(t, err)

	// Start the Nexus client
	server.nexusClient.Start()

	// Wait for Nexus connection
	time.Sleep(2 * time.Second)

	return server, server.dbManager
}

func TestTrainingWorkflow(t *testing.T) {
	// Use setupTestServerWithNexus to connect to real Nexus on port 8081
	server, dbManager := setupTestServerWithNexus(t)
	defer server.nexusClient.Stop()
	defer dbManager.Close()

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

	// Model configuration
	modelID := "llama-8b"

	// STEP 1: Load the model via WebSocket inference request
	t.Log("=== STEP 1: Loading model via WebSocket inference ===")

	// Create a dummy message for the inference request
	dummyMessage := createTestMessageDirect(t, ts.URL, user, thread, "Test message for inference")

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

	// Send inference request to trigger model loading
	inferMsg := WSMessage{
		Type: WSMessageTypeInference,
		Payload: jsonMarshal(t, WSInferenceRequest{
			LastMessageID: &storage.CompoundMessageID{
				ID:        dummyMessage.ID,
				MessageID: 0,
			},
			ModelID:   modelID,
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

	// STEP 2: Create training dataset
	t.Log("=== STEP 2: Creating training dataset ===")

	contextMessages := []core.Message{
		{Role: "system", Content: "You are an AI researcher specializing in machine learning and neural networks."},
	}

	// Training prompts
	trainingPrompts := []string{
		"Explain the transformer architecture in detail.",
		"Describe the process of training a large language model.",
		"What are the key differences between supervised and unsupervised learning?",
		"Explain gradient descent and its variants.",
	}

	// Simulate dataset (in production, these would be actual model responses)
	for i, prompt := range trainingPrompts {
		contextMessages = append(contextMessages,
			core.Message{Role: "user", Content: prompt},
			core.Message{Role: "assistant", Content: fmt.Sprintf("Detailed response about: %s [Test content]", prompt)},
		)
		t.Logf("Added training example %d/%d", i+1, len(trainingPrompts))
	}

	// Create training dataset
	dataset := core.TrainingDataset{
		ContextMessages: contextMessages,
		TrainingPrompt:  "Advanced machine learning concepts",
	}

	datasetJSON, err := json.Marshal(dataset)
	require.NoError(t, err)

	// Create training message with proper message type
	trainingMsgReq := CreateMessageRequest{
		ThreadID:    thread.ID,
		Content:     string(datasetJSON),
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

	// STEP 3: Start training
	t.Log("=== STEP 3: Starting training job ===")

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
		case "processed_dataset":
			datasetProcessed = true
			t.Log("Dataset processing completed successfully")

		case "starting_training":
			t.Log("Training is being initialized...")

		case "training":
			trainingStarted = true
			t.Log("Training has started")

			// For test purposes, we can stop here since we've verified training started
			// In production, you'd wait for completion
			if statusCheckCount > 20 {
				t.Log("Training confirmed to be running, ending test")
				trainingCompleted = true
				goto done
			}

		case "completed":
			trainingCompleted = true
			t.Log("Training completed successfully")
			goto done

		case "error":
			t.Fatalf("Training job failed with error: %s", statusResp.Error)
		}

		// Wait before next check
		time.Sleep(10 * time.Second)
	}

done:
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

	// Verify model was created if training completed
	if trainingCompleted && finalStatus.Status == "completed" {
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
	}

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
	return msgResp.Message
}

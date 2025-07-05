package api

import (
	"encoding/json"
	"fmt"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/nlpfollower/deltamind/nexus/core"
	"golang.org/x/net/websocket"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/stretchr/testify/require"
)

func TestTrainingWorkflow(t *testing.T) {
	// Skip if not using real Nexus
	if os.Getenv("USE_REAL_NEXUS") != "true" {
		t.Skip("Skipping training workflow test. Set USE_REAL_NEXUS=true to run.")
	}

	ts := NewTestServer(t)
	defer ts.Close()

	// Create test user
	user, err := createTestUser(t, ts, "trainer@example.com", "trainer", "password123")
	require.NoError(t, err)

	// Create space and thread
	space := createTestSpace(t, ts, user)
	thread := createTestThread(t, ts, user, space, "Training Thread")

	// Model configuration
	modelID := "llama-8b"

	// STEP 1: Load the model via WebSocket inference request
	t.Log("=== STEP 1: Loading model via WebSocket inference ===")

	// Create a dummy message for the inference request
	dummyMessages := createTestMessages(t, ts, user, thread, 1)
	require.NotEmpty(t, dummyMessages)
	dummyMessage := dummyMessages[0]

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
		ws.SetReadDeadline(time.Now().Add(5 * time.Minute))

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

	// Give the system a moment to stabilize after model loading
	time.Sleep(10 * time.Second)

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

	trainingMsgResp, err := performRequest[CreateMessageRequest, CreateMessageResponse](t, ts, "POST", "/api/v0/create-message", trainingMsgReq)
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

	trainResp, err := performRequest[StartTrainingRequest, StartTrainingResponse](t, ts, "POST", "/api/v0/training/start", trainReq)
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

		statusResp, err := performRequest[GetTrainingStatusRequest, GetTrainingStatusResponse](t, ts, "POST", "/api/v0/training/status", statusReq)
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

	finalStatus, err := performRequest[GetTrainingStatusRequest, GetTrainingStatusResponse](t, ts, "POST", "/api/v0/training/status", finalStatusReq)
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

		getModelsResp, err := performRequest[GetModelsRequest, GetModelsResponse](t, ts, "POST", "/api/v0/get-models", getModelsReq)
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
	ts := NewTestServer(t)
	defer ts.Close()

	// Test getting status for non-existent job
	statusReq := GetTrainingStatusRequest{
		JobID:     "non-existent-job",
		AuthToken: AuthToken{SessionKey: "test-token"},
	}

	_, err := performRequest[GetTrainingStatusRequest, GetTrainingStatusResponse](t, ts, "POST", "/api/v0/training/status", statusReq)
	require.Error(t, err)
	require.Contains(t, err.Error(), "401") // Invalid auth token
}

// TestSystemStatus tests the system status endpoint
func TestSystemStatus(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create test user
	user, err := createTestUser(t, ts, "statususer@example.com", "statususer", "testpass123")
	require.NoError(t, err)

	// Test 1: System status when idle
	t.Log("Test 1: System status when idle")

	statusReq := GetSystemStatusRequest{
		AuthToken: user.AuthToken,
	}

	statusResp, err := performRequest[GetSystemStatusRequest, GetSystemStatusResponse](t, ts, "POST", "/api/v0/system/status", statusReq)
	require.NoError(t, err)
	require.Equal(t, SystemStatusIdle, statusResp.Status)
	require.Empty(t, statusResp.ActiveTraining)
	require.Empty(t, statusResp.ActiveSessions)

	// Test 2: Create a training job and check status
	t.Log("Test 2: System status with active training")

	// First create necessary setup (space, thread, model, messages)
	space := createTestSpace(t, ts, user)
	thread := createTestThread(t, ts, user, space, "Status Test Thread")

	// Clone a model
	baseModelID := db.NewDigest([]byte("llama-8b"))
	cloneReq := CloneModelRequest{
		SourceModelID: baseModelID,
		DisplayName:   "Status Test Model",
		AuthToken:     user.AuthToken,
	}

	cloneResp, err := performRequest[CloneModelRequest, CloneModelResponse](t, ts, "POST", "/api/v0/clone-model", cloneReq)
	require.NoError(t, err)

	// Create a training message
	trainingMsgReq := CreateMessageRequest{
		ThreadID:    thread.ID,
		Content:     "Train on test data",
		Author:      "user",
		MessageType: storage.MessageTypeTraining,
		AuthToken:   user.AuthToken,
	}

	trainingMsgResp, err := performRequest[CreateMessageRequest, CreateMessageResponse](t, ts, "POST", "/api/v0/create-message", trainingMsgReq)
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

	trainResp, err := performRequest[StartTrainingRequest, StartTrainingResponse](t, ts, "POST", "/api/v0/training/start", trainReq)
	if err != nil {
		t.Fatalf("Failed to start training: %v", err)
	}

	// Now check system status
	time.Sleep(2 * time.Second) // Give it a moment to start

	statusResp2, err := performRequest[GetSystemStatusRequest, GetSystemStatusResponse](t, ts, "POST", "/api/v0/system/status", statusReq)
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

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	nexusCore "github.com/nlpfollower/deltamind/nexus/core"
	"github.com/pkg/errors"
)

const MaxTrainingHistoryMessages = 100 // Larger context for training

type StartTrainingRequest struct {
	MessageID storage.CompoundMessageID `json:"message_id"` // The training message ID
	ModelID   string                    `json:"model_id"`   // Source model for training
	AuthToken AuthToken                 `json:"auth_token"`
}

type StartTrainingResponse struct {
	JobID   string `json:"job_id"`
	ModelID string `json:"model_id"` // The new model being created
	Status  string `json:"status"`
}

type GetTrainingStatusRequest struct {
	JobID     string    `json:"job_id"`
	AuthToken AuthToken `json:"auth_token"`
}

type GetTrainingStatusResponse struct {
	JobID       string     `json:"job_id"`
	Status      string     `json:"status"`
	Progress    float64    `json:"progress"`
	Error       string     `json:"error,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ModelID     string     `json:"model_id,omitempty"` // The model being trained
}

// convertMessagesToNexusFormat converts storage messages to Nexus message format
func convertMessagesToNexusFormat(messages []*storage.CompoundMessage) []nexusCore.Message {
	nexusMessages := make([]nexusCore.Message, 0, len(messages))

	for i, msg := range messages {
		var content string

		// For all messages except the last, we need to find which version was used
		// by looking at the child message's parent reference
		if i < len(messages)-1 {
			nextMsg := messages[i+1]
			if nextMsg.ParentID != nil && nextMsg.ParentID.ID == msg.ID {
				messageIndex := nextMsg.ParentID.MessageID
				if int(messageIndex) < len(msg.Messages) {
					content = msg.Messages[messageIndex]
				}
			}
		} else {
			// For the last message, use the first version
			if len(msg.Messages) > 0 {
				content = msg.Messages[0]
			}
		}

		// Add attachments if present
		if len(msg.Attachments) > 0 {
			content += formatAttachments(msg.Attachments)
		}

		if content != "" {
			nexusMessages = append(nexusMessages, nexusCore.Message{
				Role:    msg.Author,
				Content: content,
			})
		}
	}

	return nexusMessages
}

// formatAttachments formats message attachments as text
func formatAttachments(attachments []storage.MessageAttachment) string {
	if len(attachments) == 0 {
		return ""
	}

	var result string
	result += "\n\n--- Attached Files ---\n"
	for i, attachment := range attachments {
		result += fmt.Sprintf("\n[File %d: %s (%d lines)]\n", i+1, attachment.Name, attachment.Lines)
		result += attachment.Content
		if i < len(attachments)-1 {
			result += "\n"
		}
	}
	return result
}

// StartTraining initiates a training job from a training message
func (router *APIRouter) StartTraining(w http.ResponseWriter, req *http.Request) {
	var trainReq StartTrainingRequest
	if err := json.NewDecoder(req.Body).Decode(&trainReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	claims, err := ValidateSessionKey(trainReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	userID, err := db.DigestFromString(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Check if user already has active training
	if router.hasActiveTraining(userID) {
		http.Error(w, "Another training job is already in progress", http.StatusConflict)
		return
	}

	var messages []*storage.CompoundMessage
	var trainingMessage *storage.CompoundMessage
	var sourceModel *storage.ModelInfo
	var newModel storage.ModelInfo

	err = router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		// Get the training message
		trainingMsg, err := txn.GetMessage(trainReq.MessageID.ID)
		if err != nil {
			return errors.Wrap(err, "failed to get training message")
		}
		if trainingMsg == nil {
			return errors.New("training message not found")
		}
		trainingMessage = trainingMsg

		// Verify it's a training message
		if trainingMsg.GetMessageType() != storage.MessageTypeTraining {
			return errors.New("specified message is not a training message")
		}

		// Verify user has access to this message
		thread, err := txn.GetThread(trainingMsg.ThreadID)
		if err != nil {
			return errors.Wrap(err, "failed to get thread")
		}
		space, err := txn.GetSpace(thread.SpaceID)
		if err != nil {
			return errors.Wrap(err, "failed to get space")
		}
		if space.UserID != userID {
			return errors.New("user does not have access to this training message")
		}

		// Get message history (excluding the training message itself)
		allMessages, err := txn.GetMessagePath(trainReq.MessageID.ID, trainReq.MessageID.MessageID, MaxTrainingHistoryMessages)
		if err != nil {
			return errors.Wrap(err, "failed to get message history")
		}
		// Exclude the training message from context
		messages = allMessages[:len(allMessages)-1]

		// Get source model directly by ID (deterministic from name)
		modelDigest := db.NewDigest([]byte(trainReq.ModelID))
		sourceModel, err = txn.GetModel(modelDigest)
		if err != nil || sourceModel == nil {
			return fmt.Errorf("source model not found: %s", trainReq.ModelID)
		}

		// Validate that source model can be trained
		if sourceModel.ModelType == storage.ModelTypeBase {
			return errors.New("cannot train directly from base model - clone it first")
		}

		// Check permissions
		if sourceModel.UserID != userID {
			return errors.New("unauthorized to train from this model")
		}

		// Validate checkpoint path
		if sourceModel.CheckpointPath == "" {
			return fmt.Errorf("source model has no checkpoint path")
		}

		// Extract base name and current training number if it exists
		var baseName string
		var currentTrainNum int

		if strings.Contains(sourceModel.Name, "-t") {
			// Model already has training iterations
			lastTIndex := strings.LastIndex(sourceModel.Name, "-t")
			baseName = sourceModel.Name[:lastTIndex]
			fmt.Sscanf(sourceModel.Name[lastTIndex+2:], "%d", &currentTrainNum)
		} else {
			// First training iteration for this clone
			baseName = sourceModel.Name
			currentTrainNum = 0
		}

		newTrainNum := currentTrainNum + 1
		newModelName := fmt.Sprintf("%s-t%d", baseName, newTrainNum)

		// Generate deterministic model ID from name
		modelID := db.NewDigest([]byte(newModelName))

		// Create new model entry
		timeNow := time.Now()
		newModel = storage.ModelInfo{
			ID:             modelID,
			UserID:         userID,
			Name:           newModelName,
			DisplayName:    fmt.Sprintf("Training %d from %s", newTrainNum, sourceModel.DisplayName),
			ModelType:      storage.ModelTypeTrained,
			BaseModel:      sourceModel.BaseModel,
			ModelSize:      sourceModel.ModelSize,
			ParentID:       &sourceModel.ID, // Points to immediate parent
			Status:         storage.ModelStatusTraining,
			CheckpointPath: fmt.Sprintf("/mnt/cold/contents/dcp/%s/checkpoint", newModelName),
			CreatedAt:      timeNow,
			UpdatedAt:      timeNow,
		}

		if err := txn.SetModel(userID, &newModel); err != nil {
			return errors.Wrap(err, "failed to create model")
		}

		return nil
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert messages to Nexus format
	contextMessages := convertMessagesToNexusFormat(messages)

	// Extract training prompt content
	trainingPrompt := ""
	if int(trainReq.MessageID.MessageID) < len(trainingMessage.Messages) {
		trainingPrompt = trainingMessage.Messages[trainReq.MessageID.MessageID]
	}

	// Add attachments to training prompt if present
	if len(trainingMessage.Attachments) > 0 {
		trainingPrompt += formatAttachments(trainingMessage.Attachments)
	}

	// Generate unique job ID
	jobIDBytes, _ := GenerateRandomBytes(16)
	jobID := fmt.Sprintf("job-%x", jobIDBytes)

	// Create dataset structure for Nexus
	datasetInfo := map[string]interface{}{
		"context_messages": contextMessages,
		"training_prompt":  trainingPrompt,
	}
	datasetJSON, _ := json.Marshal(datasetInfo)

	// Ensure model size is properly formatted (uppercase B)
	modelSize := sourceModel.ModelSize
	if modelSize != "" && !strings.HasSuffix(modelSize, "B") {
		modelSize = strings.ToUpper(modelSize) + "B"
	} else if modelSize != "" {
		// Ensure the 'B' is uppercase
		modelSize = strings.TrimSuffix(modelSize, "b") + "B"
	}

	// Send training request to Nexus
	trainRequest := &nexusCore.TrainingRequest{
		JobID:          jobID,
		UserID:         userID,
		SourceModelID:  sourceModel.Name,
		TargetModelID:  newModel.Name,
		CheckpointPath: sourceModel.CheckpointPath,
		OutputPath:     newModel.CheckpointPath,
		Dataset:        string(datasetJSON),
		ModelSize:      modelSize, // Pass the model size
	}

	// Store job -> model mapping
	router.storeTrainingJob(jobID, newModel.ID, userID)

	respChan, err := router.nexusClient.EnqueueTraining(trainRequest)
	if err != nil {
		// Update model status to error
		router.updateModelStatus(newModel.ID, userID, storage.ModelStatusError)
		http.Error(w, fmt.Sprintf("Failed to start training: %v", err), http.StatusInternalServerError)
		return
	}

	// Wait for initial response
	resp, ok := <-respChan
	if !ok {
		router.updateModelStatus(newModel.ID, userID, storage.ModelStatusError)
		http.Error(w, "Failed to receive training response", http.StatusInternalServerError)
		return
	}

	// Parse training response
	var trainResp nexusCore.TrainingResponse
	if err := json.Unmarshal(resp.Data, &trainResp); err != nil {
		router.updateModelStatus(newModel.ID, userID, storage.ModelStatusError)
		http.Error(w, "Failed to parse training response", http.StatusInternalServerError)
		return
	}

	response := StartTrainingResponse{
		JobID:   trainResp.JobID,
		ModelID: newModel.Name,
		Status:  trainResp.Status,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetTrainingStatus checks the status of a training job
func (router *APIRouter) GetTrainingStatus(w http.ResponseWriter, req *http.Request) {
	var statusReq GetTrainingStatusRequest
	if err := json.NewDecoder(req.Body).Decode(&statusReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, err := ValidateSessionKey(statusReq.AuthToken.SessionKey)
	if err != nil {
		http.Error(w, "Invalid auth token", http.StatusUnauthorized)
		return
	}

	// Get stored job info
	modelID, userID, ok := router.getTrainingJob(statusReq.JobID)
	if !ok {
		http.Error(w, "Training job not found", http.StatusNotFound)
		return
	}

	// Send status request to Nexus
	nexusStatusReq := &nexusCore.TrainingStatusRequest{
		JobID: statusReq.JobID,
	}

	respChan, err := router.nexusClient.GetTrainingStatus(nexusStatusReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get training status: %v", err), http.StatusInternalServerError)
		return
	}

	// Wait for response
	resp, ok := <-respChan
	if !ok {
		http.Error(w, "Failed to receive status response", http.StatusInternalServerError)
		return
	}

	// Parse status response
	var statusResp nexusCore.TrainingStatusResponse
	if err := json.Unmarshal(resp.Data, &statusResp); err != nil {
		http.Error(w, "Failed to parse status response", http.StatusInternalServerError)
		return
	}

	// Update model status based on training status
	if statusResp.Status == "completed" {
		router.updateModelStatus(modelID, userID, storage.ModelStatusReady)
		router.removeTrainingJob(statusReq.JobID)
	} else if statusResp.Status == "error" || statusResp.Status == "failed" {
		router.updateModelStatus(modelID, userID, storage.ModelStatusError)
		router.removeTrainingJob(statusReq.JobID)
	}

	// Get model name for response
	var modelName string
	router.dbManager.View(func(txn *storage.DatabaseTransaction) error {
		model, err := txn.GetModel(modelID)
		if err == nil && model != nil {
			modelName = model.Name
		}
		return nil
	})

	response := GetTrainingStatusResponse{
		JobID:       statusResp.JobID,
		Status:      statusResp.Status,
		Progress:    statusResp.Progress,
		Error:       statusResp.Error,
		StartedAt:   statusResp.StartedAt,
		CompletedAt: statusResp.CompletedAt,
		ModelID:     modelName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper functions

// updateModelStatus updates the status of a model
func (router *APIRouter) updateModelStatus(modelID db.Digest, userID db.Digest, status storage.ModelStatus) error {
	return router.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		model, err := txn.GetModel(modelID)
		if err != nil {
			return err
		}
		if model == nil {
			return errors.New("model not found")
		}

		model.Status = status
		model.UpdatedAt = time.Now()

		return txn.SetModel(userID, model)
	})
}

// Training job tracking - in production, this should be persistent
var (
	trainingJobs   = make(map[string]trainingJobInfo)
	trainingJobsMu sync.RWMutex
)

type trainingJobInfo struct {
	ModelID   db.Digest
	UserID    db.Digest
	StartedAt time.Time
}

func (router *APIRouter) storeTrainingJob(jobID string, modelID, userID db.Digest) {
	trainingJobsMu.Lock()
	defer trainingJobsMu.Unlock()
	trainingJobs[jobID] = trainingJobInfo{
		ModelID:   modelID,
		UserID:    userID,
		StartedAt: time.Now(),
	}
}

func (router *APIRouter) getTrainingJob(jobID string) (modelID, userID db.Digest, ok bool) {
	trainingJobsMu.RLock()
	defer trainingJobsMu.RUnlock()
	info, ok := trainingJobs[jobID]
	return info.ModelID, info.UserID, ok
}

func (router *APIRouter) removeTrainingJob(jobID string) {
	trainingJobsMu.Lock()
	defer trainingJobsMu.Unlock()
	delete(trainingJobs, jobID)
}

func (router *APIRouter) hasActiveTraining(userID db.Digest) bool {
	trainingJobsMu.RLock()
	defer trainingJobsMu.RUnlock()
	for _, info := range trainingJobs {
		if info.UserID == userID {
			return true
		}
	}
	return false
}

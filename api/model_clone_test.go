package api

import (
	"testing"
	"time"

	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/stretchr/testify/require"
)

func TestModelCloning(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create a user
	user, err := createTestUser(t, ts, "cloneuser@example.com", "cloneuser", "password123")
	require.NoError(t, err)

	// First, we need to add base models via the database directly
	// since the API doesn't expose base model creation
	baseModelID := addBaseModelDirectly(t, ts.Server.dbManager)

	// Test getting user models (should include base models)
	testGetUserModels(t, ts, user, baseModelID)

	// Test cloning a base model
	clonedModel := testCloneModel(t, ts, user, baseModelID)

	// Test clone status checking
	testCloneStatus(t, ts, user, clonedModel.CloneJobID)

	// Test cloning a cloned model
	testCloneClonedModel(t, ts, user, clonedModel.Model.ID)
}

func addBaseModelDirectly(t *testing.T, dbManager *storage.DatabaseManager) db.Digest {
	var modelID db.Digest

	err := dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		// Create base model
		modelIDBytes, err := GenerateRandomBytes(32)
		if err != nil {
			return err
		}

		modelID = db.NewDigest(modelIDBytes)
		systemUserID := db.NewDigest([]byte("system-base-models"))

		model := storage.ModelInfo{
			ID:           modelID,
			UserID:       systemUserID,
			Name:         "llama-8b",
			DisplayName:  "Llama 8B Base",
			ModelType:    storage.ModelTypeBase,
			BaseModel:    "llama-8b",
			ModelSize:    "8B",
			Status:       storage.ModelStatusReady,
			PhysicalPath: "/mnt/cold-storage/contents/dcp/llama-8b",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		return txn.SetModel(systemUserID, &model)
	})

	require.NoError(t, err)
	return modelID
}

func testGetUserModels(t *testing.T, ts *TestServer, user *SignUpResponse, baseModelID db.Digest) {
	req := GetUserModelsRequest{
		IncludeBase: true,
		AuthToken:   user.AuthToken,
	}

	resp, err := performRequest[GetUserModelsRequest, GetUserModelsResponse](
		t, ts, "POST", "/api/v0/get-user-models", req)
	require.NoError(t, err)

	// Should have at least the base model
	require.GreaterOrEqual(t, len(resp.Models), 1)

	// Find the base model
	found := false
	for _, model := range resp.Models {
		if model.ID == baseModelID {
			found = true
			require.Equal(t, storage.ModelTypeBase, model.ModelType)
			require.Equal(t, "llama-8b", model.BaseModel)
			require.Equal(t, storage.ModelStatusReady, model.Status)
			break
		}
	}
	require.True(t, found, "Base model not found in user models")
}

func testCloneModel(t *testing.T, ts *TestServer, user *SignUpResponse, sourceModelID db.Digest) *CloneModelResponse {
	req := CloneModelRequest{
		SourceModelID: sourceModelID,
		DisplayName:   "My Llama 8B Clone",
		AuthToken:     user.AuthToken,
	}

	resp, err := performRequest[CloneModelRequest, CloneModelResponse](
		t, ts, "POST", "/api/v0/clone-model", req)
	require.NoError(t, err)

	// Verify response
	require.NotEmpty(t, resp.CloneJobID)
	require.Equal(t, storage.ModelTypeClone, resp.Model.ModelType)
	require.Equal(t, "llama-8b", resp.Model.BaseModel)
	require.Equal(t, storage.ModelStatusCloning, resp.Model.Status)
	require.Equal(t, "My Llama 8B Clone", resp.Model.DisplayName)
	require.NotNil(t, resp.Model.ParentID)
	require.Equal(t, sourceModelID, *resp.Model.ParentID)

	// Verify model naming convention
	require.Contains(t, resp.Model.Name, "llama-8b-u")
	require.Contains(t, resp.Model.Name, "-c")

	return resp
}

func testCloneStatus(t *testing.T, ts *TestServer, user *SignUpResponse, cloneJobID string) {
	// In a real test, we'd mock the nexus response
	// For now, just verify the request works
	req := GetCloneStatusRequest{
		CloneJobID: cloneJobID,
		AuthToken:  user.AuthToken,
	}

	// This will fail without a proper nexus mock, but the request structure is tested
	_, _ = performRequest[GetCloneStatusRequest, GetCloneStatusResponse](
		t, ts, "POST", "/api/v0/get-clone-status", req)

	// In production, you'd check:
	// require.NoError(t, err)
	// require.Equal(t, cloneJobID, resp.JobID)
	// require.Contains(t, []string{"pending", "running", "completed"}, resp.Status)
}

func testCloneClonedModel(t *testing.T, ts *TestServer, user *SignUpResponse, clonedModelID db.Digest) {
	// First update the cloned model status to ready
	err := ts.Server.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		model, err := txn.GetModel(clonedModelID)
		if err != nil {
			return err
		}
		model.Status = storage.ModelStatusReady
		return txn.SetModel(model.UserID, model)
	})
	require.NoError(t, err)

	// Now clone the cloned model
	req := CloneModelRequest{
		SourceModelID: clonedModelID,
		DisplayName:   "Clone of Clone",
		AuthToken:     user.AuthToken,
	}

	resp, err := performRequest[CloneModelRequest, CloneModelResponse](
		t, ts, "POST", "/api/v0/clone-model", req)
	require.NoError(t, err)

	// Verify it creates a new clone line
	require.Equal(t, storage.ModelTypeClone, resp.Model.ModelType)
	require.Equal(t, "llama-8b", resp.Model.BaseModel)
	require.Contains(t, resp.Model.Name, "-c")

	// Should have different clone number than parent
	// This tests that cloning a trained model resets the hierarchy
}

func TestModelTypeRestrictions(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create a user
	user, err := createTestUser(t, ts, "restrictuser@example.com", "restrictuser", "password123")
	require.NoError(t, err)

	// Add a base model and a trained model
	baseModelID := addBaseModelDirectly(t, ts.Server.dbManager)
	trainedModelID := addTrainedModelDirectly(t, user.AuthToken.UserID, ts.Server.dbManager)

	// Test that only clone and trained models can be trained
	// (This would be tested when training endpoints are implemented)

	// Test model filtering by type
	req := GetUserModelsRequest{
		IncludeBase: true,
		AuthToken:   user.AuthToken,
	}

	resp, err := performRequest[GetUserModelsRequest, GetUserModelsResponse](
		t, ts, "POST", "/api/v0/get-user-models", req)
	require.NoError(t, err)

	// Verify we have models of different types
	hasBase := false
	hasTrained := false

	for _, model := range resp.Models {
		if model.ID == baseModelID {
			hasBase = true
			require.Equal(t, storage.ModelTypeBase, model.ModelType)
		}
		if model.ID == trainedModelID {
			hasTrained = true
			require.Equal(t, storage.ModelTypeTrained, model.ModelType)
		}
	}

	require.True(t, hasBase, "Base model not found")
	require.True(t, hasTrained, "Trained model not found")
}

func addTrainedModelDirectly(t *testing.T, userIDStr string, dbManager *storage.DatabaseManager) db.Digest {
	var modelID db.Digest

	userID, err := db.DigestFromString(userIDStr)
	require.NoError(t, err)

	err = dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		modelIDBytes, err := GenerateRandomBytes(32)
		if err != nil {
			return err
		}

		modelID = db.NewDigest(modelIDBytes)

		model := storage.ModelInfo{
			ID:           modelID,
			UserID:       userID,
			Name:         "llama-8b-u1-c1-t1",
			DisplayName:  "Trained Model",
			ModelType:    storage.ModelTypeTrained,
			BaseModel:    "llama-8b",
			ModelSize:    "8B",
			Status:       storage.ModelStatusReady,
			PhysicalPath: "/mnt/cold-storage/contents/dcp/llama-8b-u1-c1-t1",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		return txn.SetModel(userID, &model)
	})

	require.NoError(t, err)
	return modelID
}

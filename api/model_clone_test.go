package api

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
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

	// Add base models
	baseModel8B := addBaseModelDirectly(t, ts.Server.dbManager, "llama-8b", "8B")
	baseModel70B := addBaseModelDirectly(t, ts.Server.dbManager, "llama-70b", "70B")

	// Test cloning a base model
	t.Run("CloneBaseModel", func(t *testing.T) {
		req := CloneModelRequest{
			SourceModelID: baseModel8B,
			DisplayName:   "My Llama 8B Clone",
			AuthToken:     user.AuthToken,
		}

		resp, err := performRequest[CloneModelRequest, CloneModelResponse](
			t, ts, "POST", "/api/v0/clone-model", req)
		require.NoError(t, err)

		// Verify response
		require.Equal(t, storage.ModelTypeClone, resp.Model.ModelType)
		require.Equal(t, "llama-8b", resp.Model.BaseModel)
		require.Equal(t, storage.ModelStatusReady, resp.Model.Status)
		require.Equal(t, "My Llama 8B Clone", resp.Model.DisplayName)
		require.NotNil(t, resp.Model.ParentID)
		require.Equal(t, baseModel8B, *resp.Model.ParentID)

		// Verify model naming convention (first clone is c0)
		userIDStr, _ := db.DigestFromString(user.AuthToken.UserID)
		expectedName := fmt.Sprintf("llama-8b-u%s-c0", userIDStr.String()[:8])
		require.Equal(t, expectedName, resp.Model.Name)

		// Verify checkpoint path was inherited
		require.Equal(t, "/mnt/cold/contents/dcp/llama-8b/checkpoint", resp.Model.CheckpointPath)
	})

	// Test clone numbering increments properly
	t.Run("CloneNumberIncrement", func(t *testing.T) {
		// Clone again (should be c1)
		req := CloneModelRequest{
			SourceModelID: baseModel8B,
			DisplayName:   "My Second Clone",
			AuthToken:     user.AuthToken,
		}

		resp, err := performRequest[CloneModelRequest, CloneModelResponse](
			t, ts, "POST", "/api/v0/clone-model", req)
		require.NoError(t, err)

		userIDStr, _ := db.DigestFromString(user.AuthToken.UserID)
		expectedName := fmt.Sprintf("llama-8b-u%s-c1", userIDStr.String()[:8])
		require.Equal(t, expectedName, resp.Model.Name)

		// Clone the 70B model (should be c2)
		req = CloneModelRequest{
			SourceModelID: baseModel70B,
			DisplayName:   "My 70B Clone",
			AuthToken:     user.AuthToken,
		}

		resp, err = performRequest[CloneModelRequest, CloneModelResponse](
			t, ts, "POST", "/api/v0/clone-model", req)
		require.NoError(t, err)

		// Clone numbers are per-user, not per-model
		expectedName = fmt.Sprintf("llama-70b-u%s-c2", userIDStr.String()[:8])
		require.Equal(t, expectedName, resp.Model.Name)
	})

	// Test listing models
	t.Run("ListModels", func(t *testing.T) {
		req := GetModelsRequest{
			AuthToken:   user.AuthToken,
			IncludeBase: true,
		}

		resp, err := performRequest[GetModelsRequest, GetModelsResponse](
			t, ts, "POST", "/api/v0/get-models", req)
		require.NoError(t, err)

		// Should have 3 user models + 2 base models
		require.GreaterOrEqual(t, len(resp.Models), 5)

		// Count model types
		var baseCount, cloneCount int
		var userModels []*storage.ModelInfo

		for _, model := range resp.Models {
			switch model.ModelType {
			case storage.ModelTypeBase:
				baseCount++
			case storage.ModelTypeClone:
				cloneCount++
				userID, _ := db.DigestFromString(user.AuthToken.UserID)
				if model.UserID == userID {
					userModels = append(userModels, model)
				}
			}
		}

		require.GreaterOrEqual(t, baseCount, 2) // At least our 2 base models
		require.Equal(t, 3, len(userModels))    // Exactly 3 clones for our user
	})

	// Test cloning without permission
	t.Run("CloneUnauthorized", func(t *testing.T) {
		// Create another user
		user2, err := createTestUser(t, ts, "user2@example.com", "user2", "password123")
		require.NoError(t, err)

		// Get user1's model
		var user1ModelID string
		err = ts.Server.dbManager.View(func(txn *storage.DatabaseTransaction) error {
			userID, _ := db.DigestFromString(user.AuthToken.UserID)
			models, err := txn.GetUserModels(userID, 10, uint64(time.Now().UnixNano()))
			if err != nil {
				return err
			}
			if len(models) > 0 {
				user1ModelID = string(models[0].ID.Bytes())
			}
			return nil
		})
		require.NoError(t, err)

		// Try to clone user1's model as user2
		req := CloneModelRequest{
			SourceModelID: user1ModelID,
			DisplayName:   "Stolen Clone",
			AuthToken:     user2.AuthToken,
		}

		_, err = performRequest[CloneModelRequest, CloneModelResponse](
			t, ts, "POST", "/api/v0/clone-model", req)
		require.Error(t, err)
	})
}

func TestCheckpointPathInheritance(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	user, err := createTestUser(t, ts, "pathuser@example.com", "pathuser", "password123")
	require.NoError(t, err)

	// Create a chain: base -> clone -> trained -> clone
	baseModelID := addBaseModelDirectly(t, ts.Server.dbManager, "llama-8b", "8B")

	// Clone base (c0)
	clone1Req := CloneModelRequest{
		SourceModelID: baseModelID,
		DisplayName:   "Clone 1",
		AuthToken:     user.AuthToken,
	}
	clone1Resp, err := performRequest[CloneModelRequest, CloneModelResponse](
		t, ts, "POST", "/api/v0/clone-model", clone1Req)
	require.NoError(t, err)
	require.Equal(t, "/mnt/cold/contents/dcp/llama-8b/checkpoint", clone1Resp.Model.CheckpointPath)

	// Verify name
	userID, _ := db.DigestFromString(user.AuthToken.UserID)
	require.Equal(t, fmt.Sprintf("llama-8b-u%s-c0", userID.String()[:8]), clone1Resp.Model.Name)

	// Simulate training completion (manually create trained model)
	var modelIDStr string
	err = ts.Server.dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		modelIDStr, _ := GenerateRandomModelID()
		trainedModelID := db.NewDigest([]byte(modelIDStr))

		trained := storage.ModelInfo{
			ID:             trainedModelID,
			UserID:         userID,
			Name:           fmt.Sprintf("llama-8b-u%s-c0-t1", userID.String()[:8]),
			DisplayName:    "Trained Model",
			ModelType:      storage.ModelTypeTrained,
			BaseModel:      "llama-8b",
			ModelSize:      "8B",
			ParentID:       &clone1Resp.Model.ID,
			Status:         storage.ModelStatusReady,
			CheckpointPath: fmt.Sprintf("/mnt/cold/contents/dcp/llama-8b-u%s-c0-t1/checkpoint", userID.String()[:8]),
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		return txn.SetModel(userID, &trained)
	})
	require.NoError(t, err)

	// Clone the trained model (should be c1)
	clone2Req := CloneModelRequest{
		SourceModelID: modelIDStr,
		DisplayName:   "Clone of Trained",
		AuthToken:     user.AuthToken,
	}
	clone2Resp, err := performRequest[CloneModelRequest, CloneModelResponse](
		t, ts, "POST", "/api/v0/clone-model", clone2Req)
	require.NoError(t, err)

	// Should inherit the trained model's checkpoint, not create a chain
	expectedCheckpoint := fmt.Sprintf("/mnt/cold/contents/dcp/llama-8b-u%s-c0-t1/checkpoint", userID.String()[:8])
	require.Equal(t, expectedCheckpoint, clone2Resp.Model.CheckpointPath)

	// Should be c1 (next clone number)
	expectedName := fmt.Sprintf("llama-8b-u%s-c1", userID.String()[:8])
	require.Equal(t, expectedName, clone2Resp.Model.Name)
}

func TestMultipleUsersCloning(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create base model
	baseModelID := addBaseModelDirectly(t, ts.Server.dbManager, "llama-8b", "8B")

	// Create multiple users
	users := []struct {
		email    string
		username string
	}{
		{"user1@example.com", "user1"},
		{"user2@example.com", "user2"},
		{"user3@example.com", "user3"},
	}

	for _, u := range users {
		user, err := createTestUser(t, ts, u.email, u.username, "password123")
		require.NoError(t, err)

		// Each user clones the base model
		req := CloneModelRequest{
			SourceModelID: baseModelID,
			DisplayName:   fmt.Sprintf("%s's Clone", u.username),
			AuthToken:     user.AuthToken,
		}

		resp, err := performRequest[CloneModelRequest, CloneModelResponse](
			t, ts, "POST", "/api/v0/clone-model", req)
		require.NoError(t, err)

		// Each user's first clone should be c0
		userID, _ := db.DigestFromString(user.AuthToken.UserID)
		expectedName := fmt.Sprintf("llama-8b-u%s-c0", userID.String()[:8])
		require.Equal(t, expectedName, resp.Model.Name)

		// Clone again for the same user
		req.DisplayName = fmt.Sprintf("%s's Second Clone", u.username)
		resp, err = performRequest[CloneModelRequest, CloneModelResponse](
			t, ts, "POST", "/api/v0/clone-model", req)
		require.NoError(t, err)

		// Should be c1
		expectedName = fmt.Sprintf("llama-8b-u%s-c1", userID.String()[:8])
		require.Equal(t, expectedName, resp.Model.Name)
	}
}

func TestParentChildRelationships(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	user, err := createTestUser(t, ts, "parenttest@example.com", "parenttest", "password123")
	require.NoError(t, err)

	baseModelID := addBaseModelDirectly(t, ts.Server.dbManager, "llama-70b", "70B")

	// Create a tree of clones
	var cloneIDs []db.Digest

	// Clone from base
	for i := 0; i < 3; i++ {
		req := CloneModelRequest{
			SourceModelID: baseModelID,
			DisplayName:   fmt.Sprintf("Clone %d from base", i),
			AuthToken:     user.AuthToken,
		}
		resp, err := performRequest[CloneModelRequest, CloneModelResponse](
			t, ts, "POST", "/api/v0/clone-model", req)
		require.NoError(t, err)
		cloneIDs = append(cloneIDs, resp.Model.ID)
	}

	// Test parent-child queries
	err = ts.Server.dbManager.View(func(txn *storage.DatabaseTransaction) error {
		// Get children of base model
		children, err := txn.GetModelsByParent(db.NewDigest([]byte(baseModelID)), 10, uint64(time.Now().Add(time.Hour).UnixNano()))
		require.NoError(t, err)
		require.Len(t, children, 3)

		// Verify all children point to base
		for _, child := range children {
			require.NotNil(t, child.ParentID)
			require.Equal(t, baseModelID, *child.ParentID)
			require.Equal(t, storage.ModelTypeClone, child.ModelType)
		}

		return nil
	})
	require.NoError(t, err)
}

func TestResolveCheckpointPath(t *testing.T) {
	// Test with valid checkpoint
	model := &storage.ModelInfo{
		Name:           "test-model",
		CheckpointPath: "/mnt/storage/checkpoint",
	}

	path, err := ResolveCheckpointPath(model)
	require.NoError(t, err)
	require.Equal(t, "/mnt/storage/checkpoint", path)

	// Test with missing checkpoint
	model.CheckpointPath = ""
	_, err = ResolveCheckpointPath(model)
	require.Error(t, err)
	require.Contains(t, err.Error(), "has no checkpoint path")
}

func GenerateRandomModelID() (string, error) {
	const max = 1 << 20 // 1,048,576

	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	n := binary.BigEndian.Uint32(b[:]) % max
	return fmt.Sprintf("model-%d", n), nil
}

func addBaseModelDirectly(t *testing.T, dbManager *storage.DatabaseManager, modelName, modelSize string) string {
	var modelIDStr string

	err := dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		modelIDStr, err := GenerateRandomModelID()
		if err != nil {
			return err
		}

		modelID := db.NewDigest([]byte(modelIDStr))
		systemUserID := db.NewDigest([]byte("system-base-models"))

		model := storage.ModelInfo{
			ID:             modelID,
			UserID:         systemUserID,
			Name:           modelName,
			DisplayName:    fmt.Sprintf("%s Base", modelName),
			ModelType:      storage.ModelTypeBase,
			BaseModel:      modelName,
			ModelSize:      modelSize,
			Status:         storage.ModelStatusReady,
			CheckpointPath: fmt.Sprintf("/mnt/cold/contents/dcp/%s/checkpoint", modelName),
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		return txn.SetModel(systemUserID, &model)
	})

	require.NoError(t, err)
	return modelIDStr
}

package api

import (
	"fmt"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/stretchr/testify/require"
	"math"
	"testing"
	"time"
)

func TestModelCreationAndRetrieval(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create a user
	user, err := createTestUser(t, ts, "user@example.com", "testuser", "password123")
	require.NoError(t, err)

	// Create 5 models
	createdModels := createTestModels(t, ts, user, 5)

	// Test model retrieval
	testModelRetrieval(t, ts, user, createdModels)

	// Test pagination
	testModelPagination(t, ts, user)

	// Test model deletion
	testModelDeletion(t, ts, user, createdModels[0])

	// Test model iteration creation and retrieval
	testModelIterations(t, ts, user, createdModels[1])
}

func testModelIterations(t *testing.T, ts *TestServer, user *SignUpResponse, model *storage.ModelInfo) {
	// Create 3 iterations for the model
	createdIterations := createTestModelIterations(t, ts, user, model, 3)

	// Test iteration retrieval
	testModelIterationRetrieval(t, ts, user, model, createdIterations)
}

func createTestModels(t *testing.T, ts *TestServer, user *SignUpResponse, count int) []*storage.ModelInfo {
	var models []*storage.ModelInfo
	for i := 0; i < count; i++ {
		createModelReq := CreateModelRequest{
			Name:      fmt.Sprintf("Test Model %d", i+1),
			AuthToken: user.AuthToken,
		}
		createModelResp, err := performRequest[CreateModelRequest, CreateModelResponse](t, ts, "POST", "/api/v0/create-model", createModelReq)
		require.NoError(t, err)
		require.NotNil(t, createModelResp.Model)
		models = append(models, &createModelResp.Model)
		// Add a small delay to ensure unique timestamps
		time.Sleep(time.Millisecond)
	}
	return models
}

func testModelRetrieval(t *testing.T, ts *TestServer, user *SignUpResponse, createdModels []*storage.ModelInfo) {
	getModelsReq := GetModelsRequest{
		Limit:        len(createdModels),
		MaxTimestamp: uint64(time.Now().Add(time.Hour).UnixNano()), // Use a future timestamp to get all models
		AuthToken:    user.AuthToken,
	}
	getModelsResp, err := performRequest[GetModelsRequest, GetModelsResponse](t, ts, "POST", "/api/v0/get-models", getModelsReq)
	require.NoError(t, err)
	require.Len(t, getModelsResp.Models, len(createdModels))

	// Verify that all models are retrieved in reverse order
	for i, model := range getModelsResp.Models {
		require.Equal(t, createdModels[len(createdModels)-1-i].ID, model.ID)
		require.Equal(t, createdModels[len(createdModels)-1-i].Name, model.Name)
	}

	// Verify NextMaxTimestamp
	require.NotNil(t, getModelsResp.NextMaxTimestamp)
	require.Equal(t, uint64(createdModels[0].UpdatedAt.UnixNano()), *getModelsResp.NextMaxTimestamp)
}

func testModelPagination(t *testing.T, ts *TestServer, user *SignUpResponse) {
	pageSize := 2
	totalPages := 3

	var allModels []*storage.ModelInfo
	var maxTimestamp uint64 = uint64(time.Now().Add(time.Hour).UnixNano())

	for page := 0; page < totalPages; page++ {
		getModelsReq := GetModelsRequest{
			Limit:        pageSize,
			MaxTimestamp: maxTimestamp,
			AuthToken:    user.AuthToken,
		}
		getModelsResp, err := performRequest[GetModelsRequest, GetModelsResponse](t, ts, "POST", "/api/v0/get-models", getModelsReq)
		require.NoError(t, err)

		if page < totalPages-1 {
			require.Len(t, getModelsResp.Models, pageSize)
		} else {
			// On the last page, we expect the remaining models
			remainingModels := 5 - (pageSize * (totalPages - 1))
			require.Len(t, getModelsResp.Models, remainingModels)
		}

		allModels = append(allModels, getModelsResp.Models...)

		// Update maxTimestamp for the next iteration
		if getModelsResp.NextMaxTimestamp != nil {
			maxTimestamp = *getModelsResp.NextMaxTimestamp
		} else {
			break // No more models to fetch
		}
	}

	// Verify that we've retrieved all 5 models
	require.Len(t, allModels, 5)

	// Verify that all models are unique and in descending order of creation
	modelMap := make(map[string]bool)
	var lastTimestamp time.Time
	for _, model := range allModels {
		require.False(t, modelMap[model.ID.String()], "Duplicate model found")
		modelMap[model.ID.String()] = true

		if !lastTimestamp.IsZero() {
			require.True(t, model.UpdatedAt.Before(lastTimestamp) || model.UpdatedAt.Equal(lastTimestamp), "Models not in descending order")
		}
		lastTimestamp = model.UpdatedAt
	}
}

func testModelDeletion(t *testing.T, ts *TestServer, user *SignUpResponse, modelToDelete *storage.ModelInfo) {
	deleteModelReq := DeleteModelRequest{
		ModelID:   modelToDelete.ID,
		AuthToken: user.AuthToken,
	}
	deleteModelResp, err := performRequest[DeleteModelRequest, DeleteModelResponse](t, ts, "POST", "/api/v0/delete-model", deleteModelReq)
	require.NoError(t, err)
	require.True(t, deleteModelResp.Success)

	// Verify the model was deleted
	getModelsReq := GetModelsRequest{
		Limit:        10,
		MaxTimestamp: uint64(time.Now().Add(time.Hour).UnixNano()),
		AuthToken:    user.AuthToken,
	}
	getModelsResp, err := performRequest[GetModelsRequest, GetModelsResponse](t, ts, "POST", "/api/v0/get-models", getModelsReq)
	require.NoError(t, err)

	for _, model := range getModelsResp.Models {
		require.NotEqual(t, modelToDelete.ID, model.ID, "Deleted model should not be retrieved")
	}
}

func createTestModelIterations(t *testing.T, ts *TestServer, user *SignUpResponse, model *storage.ModelInfo, count int) []*storage.ModelIteration {
	var iterations []*storage.ModelIteration
	for i := 0; i < count; i++ {
		createIterationReq := CreateModelIterationRequest{
			ModelID:     model.ID,
			Description: fmt.Sprintf("Iteration %d for %s", i+1, model.Name),
			AuthToken:   user.AuthToken,
		}
		createIterationResp, err := performRequest[CreateModelIterationRequest, CreateModelIterationResponse](t, ts, "POST", "/api/v0/create-model-iteration", createIterationReq)
		require.NoError(t, err)
		require.NotNil(t, createIterationResp.Iteration)
		iterations = append(iterations, &createIterationResp.Iteration)
		// Add a small delay to ensure unique timestamps
		time.Sleep(time.Millisecond)
	}
	return iterations
}

func testModelIterationRetrieval(t *testing.T, ts *TestServer, user *SignUpResponse, model *storage.ModelInfo, createdIterations []*storage.ModelIteration) {
	getIterationsReq := GetModelIterationsRequest{
		ModelID:   model.ID,
		MaxIndex:  math.MaxUint64,
		Limit:     uint64(len(createdIterations)),
		AuthToken: user.AuthToken,
	}
	getIterationsResp, err := performRequest[GetModelIterationsRequest, GetModelIterationsResponse](t, ts, "POST", "/api/v0/get-model-iterations", getIterationsReq)
	require.NoError(t, err)
	require.Len(t, getIterationsResp.Iterations, len(createdIterations))

	// Verify that all iterations are retrieved in the correct order
	// Reverse the order of created iterations so they match the order in the response
	for i, j := 0, len(createdIterations)-1; i < j; i, j = i+1, j-1 {
		createdIterations[i], createdIterations[j] = createdIterations[j], createdIterations[i]
	}

	for i, iteration := range getIterationsResp.Iterations {
		require.Equal(t, createdIterations[i].ID, iteration.ID)
		require.Equal(t, createdIterations[i].Description, iteration.Description)
		require.Equal(t, createdIterations[i].Index, iteration.Index)
	}

	// Test pagination
	pageSize := uint64(2)
	getIterationsReq.Limit = pageSize
	getIterationsResp, err = performRequest[GetModelIterationsRequest, GetModelIterationsResponse](t, ts, "POST", "/api/v0/get-model-iterations", getIterationsReq)
	require.NoError(t, err)
	require.Len(t, getIterationsResp.Iterations, int(pageSize))

	// Verify the order of paginated results
	for i, iteration := range getIterationsResp.Iterations {
		require.Equal(t, createdIterations[i].ID, iteration.ID)
		require.Equal(t, createdIterations[i].Description, iteration.Description)
		require.Equal(t, createdIterations[i].Index, iteration.Index)
	}
}

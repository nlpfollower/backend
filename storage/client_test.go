package storage

import (
	"fmt"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestTimestamp(t *testing.T) {
	now := time.Now()
	ts := NewTimestamp(now)
	require.Equal(t, now.UnixNano(), int64(ts.Uint64()))
	require.Equal(t, now.Unix(), ts.Time().Unix())

	value := uint64(1234567890)
	ts = NewTimestampFromUint64(value)
	require.Equal(t, value, ts.Uint64())
}

func TestGetUserSpacesReverse(t *testing.T) {
	client := NewDatabaseClient(t.TempDir())
	require.NoError(t, client.Setup())
	defer client.Close()

	userID := NewDigest([]byte("user1"))
	spaces := createTestSpaces(t, client, userID, 10)

	var result []*Space
	require.NoError(t, client.View(func(txn db.Transaction) error {
		var err error
		result, err = client.GetUserSpacesReverse(txn, userID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 5)
		return err
	}))
	require.Len(t, result, 5)
	for i := 0; i < 5; i++ {
		require.Equal(t, spaces[9-i].ID, result[i].ID)
		require.Equal(t, spaces[9-i].Name, result[i].Name)
	}
}

func TestGetSpaceThreadsReverse(t *testing.T) {
	client := NewDatabaseClient(t.TempDir())
	require.NoError(t, client.Setup())
	defer client.Close()

	spaceID := NewDigest([]byte("space1"))
	threads := createTestThreads(t, client, spaceID, 10)

	var result []*Thread
	require.NoError(t, client.View(func(txn db.Transaction) error {
		var err error
		result, err = client.GetSpaceThreadsReverse(txn, spaceID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 5)
		return err
	}))
	require.Len(t, result, 5)
	for i := 0; i < 5; i++ {
		require.Equal(t, threads[9-i].ID, result[i].ID)
		require.Equal(t, threads[9-i].Title, result[i].Title)
	}
}

func TestGetThreadMessagesReverse(t *testing.T) {
	client := NewDatabaseClient(t.TempDir())
	require.NoError(t, client.Setup())
	defer client.Close()

	threadID := NewDigest([]byte("thread1"))
	messages := createTestMessages(t, client, threadID, 10)

	var result []*CompoundMessage
	require.NoError(t, client.View(func(txn db.Transaction) error {
		var err error
		result, err = client.GetThreadMessagesReverse(txn, threadID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 5)
		return err
	}))
	require.Len(t, result, 5)
	for i := 0; i < 5; i++ {
		require.Equal(t, messages[9-i].ID, result[i].ID)
		require.Equal(t, messages[9-i].Messages[0], result[i].Messages[0])
	}
}

func TestGetUserModelsReverse(t *testing.T) {
	client := NewDatabaseClient(t.TempDir())
	require.NoError(t, client.Setup())
	defer client.Close()

	userID := NewDigest([]byte("user1"))
	models := createTestModels(t, client, userID, 10)

	var result []*ModelInfo
	require.NoError(t, client.View(func(txn db.Transaction) error {
		var err error
		result, err = client.GetUserModelsReverse(txn, userID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 5)
		return err
	}))
	require.Len(t, result, 5)
	for i := 0; i < 5; i++ {
		require.Equal(t, models[9-i].ID, result[i].ID)
		require.Equal(t, models[9-i].Name, result[i].Name)
	}
}

func TestGetModelIterations(t *testing.T) {
	client := NewDatabaseClient(t.TempDir())
	require.NoError(t, client.Setup())
	defer client.Close()

	modelID := NewDigest([]byte("model1"))
	iterations := createTestModelIterations(t, client, modelID, 10)

	var result []*ModelIteration
	require.NoError(t, client.View(func(txn db.Transaction) error {
		var err error
		result, err = client.GetModelIterationsReverse(txn, modelID.Bytes(), 10, 5)
		return err
	}))
	require.Len(t, result, 5)
	// Reverse the order of created iterations so they match the order in the response
	for i, j := 0, len(iterations)-1; i < j; i, j = i+1, j-1 {
		iterations[i], iterations[j] = iterations[j], iterations[i]
	}

	for i := 0; i < 5; i++ {
		require.Equal(t, iterations[i].ID, result[i].ID)
		require.Equal(t, iterations[i].Description, result[i].Description)
		require.Equal(t, iterations[i].Index, result[i].Index)
	}
}

func TestSecondaryIndexes(t *testing.T) {
	client := NewDatabaseClient(t.TempDir())
	require.NoError(t, client.Setup())
	defer client.Close()

	userID := NewDigest([]byte("user1"))
	spaceID := NewDigest([]byte("space1"))
	threadID := NewDigest([]byte("thread1"))
	modelID := NewDigest([]byte("model1"))

	// Create test data
	spaces := createTestSpaces(t, client, userID, 5)
	threads := createTestThreads(t, client, spaceID, 5)
	messages := createTestMessages(t, client, threadID, 5)
	models := createTestModels(t, client, userID, 5)
	iterations := createTestModelIterations(t, client, modelID, 5)
	// Reverse the order of created iterations so they match the order in the response
	for i, j := 0, len(iterations)-1; i < j; i, j = i+1, j-1 {
		iterations[i], iterations[j] = iterations[j], iterations[i]
	}

	t.Run("GetUserSpacesReverse", func(t *testing.T) {
		var result []*Space
		require.NoError(t, client.View(func(txn db.Transaction) error {
			var err error
			result, err = client.GetUserSpacesReverse(txn, userID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 3)
			return err
		}))
		require.Len(t, result, 3)
		for i := 0; i < 3; i++ {
			require.Equal(t, spaces[4-i].ID, result[i].ID)
			require.Equal(t, spaces[4-i].Name, result[i].Name)
		}
	})

	t.Run("GetSpaceThreadsReverse", func(t *testing.T) {
		var result []*Thread
		require.NoError(t, client.View(func(txn db.Transaction) error {
			var err error
			result, err = client.GetSpaceThreadsReverse(txn, spaceID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 3)
			return err
		}))
		require.Len(t, result, 3)
		for i := 0; i < 3; i++ {
			require.Equal(t, threads[4-i].ID, result[i].ID)
			require.Equal(t, threads[4-i].Title, result[i].Title)
		}
	})

	t.Run("GetThreadMessagesReverse", func(t *testing.T) {
		var result []*CompoundMessage
		require.NoError(t, client.View(func(txn db.Transaction) error {
			var err error
			result, err = client.GetThreadMessagesReverse(txn, threadID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 3)
			return err
		}))
		require.Len(t, result, 3)
		for i := 0; i < 3; i++ {
			require.Equal(t, messages[4-i].ID, result[i].ID)
			require.Equal(t, messages[4-i].Messages[0], result[i].Messages[0])
		}
	})

	t.Run("GetUserModelsReverse", func(t *testing.T) {
		var result []*ModelInfo
		require.NoError(t, client.View(func(txn db.Transaction) error {
			var err error
			result, err = client.GetUserModelsReverse(txn, userID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 3)
			return err
		}))
		require.Len(t, result, 3)
		for i := 0; i < 3; i++ {
			require.Equal(t, models[4-i].ID, result[i].ID)
			require.Equal(t, models[4-i].Name, result[i].Name)
		}
	})

	t.Run("GetModelIterations", func(t *testing.T) {
		var result []*ModelIteration
		require.NoError(t, client.View(func(txn db.Transaction) error {
			var err error
			result, err = client.GetModelIterationsReverse(txn, modelID.Bytes(), 10, 3)
			return err
		}))
		require.Len(t, result, 3)
		for i := 0; i < 3; i++ {
			require.Equal(t, iterations[i].ID, result[i].ID)
			require.Equal(t, iterations[i].Description, result[i].Description)
			require.Equal(t, iterations[i].Index, result[i].Index)
		}
	})

	t.Run("DeleteUserSpace", func(t *testing.T) {
		var result []*Space
		require.NoError(t, client.Update(func(txn db.Transaction) error {
			err := client.DeleteUserSpace(txn, userID, spaces[2].UpdatedAt)
			require.NoError(t, err)

			result, err = client.GetUserSpacesReverse(txn, userID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 5)
			return err
		}))
		require.Len(t, result, 4)
		require.NotEqual(t, spaces[2].ID, result[2].ID)
	})

	t.Run("DeleteSpaceThread", func(t *testing.T) {
		var result []*Thread
		require.NoError(t, client.Update(func(txn db.Transaction) error {
			err := client.DeleteSpaceThread(txn, spaceID, threads[2].UpdatedAt)
			require.NoError(t, err)

			result, err = client.GetSpaceThreadsReverse(txn, spaceID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 5)
			return err
		}))
		require.Len(t, result, 4)
		require.NotEqual(t, threads[2].ID, result[2].ID)
	})

	t.Run("DeleteUserModel", func(t *testing.T) {
		var result []*ModelInfo
		require.NoError(t, client.Update(func(txn db.Transaction) error {
			err := client.DeleteUserModel(txn, userID, models[2].UpdatedAt)
			require.NoError(t, err)

			result, err = client.GetUserModelsReverse(txn, userID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 5)
			return err
		}))
		require.Len(t, result, 4)
		require.NotEqual(t, models[2].ID, result[2].ID)
	})

	t.Run("DeleteModelIteration", func(t *testing.T) {
		var result []*ModelIteration
		require.NoError(t, client.Update(func(txn db.Transaction) error {
			err := client.DeleteModelIteration(txn, modelID, 2)
			require.NoError(t, err)

			result, err = client.GetModelIterationsReverse(txn, modelID.Bytes(), 10, 5)
			return err
		}))
		require.Len(t, result, 4)
		require.NotEqual(t, iterations[2].ID, result[2].ID)
	})
}

// Helper functions

func createTestSpaces(t *testing.T, client *DatabaseClient, userID Digest, count int) []*Space {
	spaces := make([]*Space, count)
	require.NoError(t, client.Update(func(txn db.Transaction) error {
		for i := 0; i < count; i++ {
			space := &Space{
				ID:          NewDigest([]byte(fmt.Sprintf("space%d", i))),
				UserID:      userID,
				Name:        fmt.Sprintf("Space %d", i),
				Description: fmt.Sprintf("Description for Space %d", i),
				CreatedAt:   time.Now().Add(time.Duration(i) * time.Minute),
				UpdatedAt:   time.Now().Add(time.Duration(i) * time.Minute),
			}
			err := client.SetSpace(txn, space)
			require.NoError(t, err)
			err = client.SetUserSpace(txn, userID, space.UpdatedAt, space)
			require.NoError(t, err)
			spaces[i] = space
		}
		return nil
	}))
	return spaces
}

func createTestThreads(t *testing.T, client *DatabaseClient, spaceID Digest, count int) []*Thread {
	threads := make([]*Thread, count)
	require.NoError(t, client.Update(func(txn db.Transaction) error {
		for i := 0; i < count; i++ {
			thread := &Thread{
				ID:        NewDigest([]byte(fmt.Sprintf("thread%d", i))),
				SpaceID:   spaceID,
				Title:     fmt.Sprintf("Thread %d", i),
				CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
				UpdatedAt: time.Now().Add(time.Duration(i) * time.Minute),
			}
			err := client.SetThread(txn, thread)
			require.NoError(t, err)
			err = client.SetSpaceThread(txn, spaceID, thread.UpdatedAt, thread)
			require.NoError(t, err)
			threads[i] = thread
		}
		return nil
	}))
	return threads
}

func createTestMessages(t *testing.T, client *DatabaseClient, threadID Digest, count int) []*CompoundMessage {
	messages := make([]*CompoundMessage, count)
	require.NoError(t, client.Update(func(txn db.Transaction) error {
		for i := 0; i < count; i++ {
			message := &CompoundMessage{
				ID:        NewDigest([]byte(fmt.Sprintf("message%d", i))),
				ThreadID:  threadID,
				Messages:  []string{fmt.Sprintf("Message %d content", i)},
				Author:    "TestUser",
				CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
				UpdatedAt: time.Now().Add(time.Duration(i) * time.Minute),
			}
			err := client.SetMessage(txn, message)
			require.NoError(t, err)
			err = client.SetThreadMessage(txn, threadID, message.CreatedAt, message)
			require.NoError(t, err)
			messages[i] = message
		}
		return nil
	}))
	return messages
}

func createTestModels(t *testing.T, client *DatabaseClient, userID Digest, count int) []*ModelInfo {
	models := make([]*ModelInfo, count)
	require.NoError(t, client.Update(func(txn db.Transaction) error {
		for i := 0; i < count; i++ {
			model := &ModelInfo{
				ID:        NewDigest([]byte(fmt.Sprintf("model%d", i))),
				UserID:    userID,
				Name:      fmt.Sprintf("Model %d", i),
				CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
				UpdatedAt: time.Now().Add(time.Duration(i) * time.Minute),
			}
			err := client.SetModel(txn, model)
			require.NoError(t, err)
			err = client.SetUserModel(txn, userID, model.UpdatedAt, model)
			require.NoError(t, err)
			models[i] = model
		}
		return nil
	}))
	return models
}

func createTestModelIterations(t *testing.T, client *DatabaseClient, modelID Digest, count int) []*ModelIteration {
	iterations := make([]*ModelIteration, count)
	require.NoError(t, client.Update(func(txn db.Transaction) error {
		for i := 0; i < count; i++ {
			iteration := &ModelIteration{
				ID:          NewDigest([]byte(fmt.Sprintf("iteration%d", i))),
				ModelID:     modelID,
				Description: fmt.Sprintf("Iteration %d", i),
				CreatedAt:   time.Now().Add(time.Duration(i) * time.Minute),
				Index:       uint64(i),
			}
			err := client.SetModelIteration(txn, modelID, uint64(i), iteration)
			require.NoError(t, err)
			iterations[i] = iteration
		}
		return nil
	}))
	return iterations
}

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

	userID := db.NewDigest([]byte("user1"))
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

	spaceID := db.NewDigest([]byte("space1"))
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

	threadID := db.NewDigest([]byte("thread1"))
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

	userID := db.NewDigest([]byte("user1"))
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

func TestSecondaryIndexes(t *testing.T) {
	client := NewDatabaseClient(t.TempDir())
	require.NoError(t, client.Setup())
	defer client.Close()

	userID := db.NewDigest([]byte("user1"))
	spaceID := db.NewDigest([]byte("space1"))
	threadID := db.NewDigest([]byte("thread1"))

	// Create test data
	spaces := createTestSpaces(t, client, userID, 5)
	threads := createTestThreads(t, client, spaceID, 5)
	messages := createTestMessages(t, client, threadID, 5)
	models := createTestModels(t, client, userID, 5)

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
}

// Helper functions

func createTestSpaces(t *testing.T, client *DatabaseClient, userID db.Digest, count int) []*Space {
	spaces := make([]*Space, count)
	require.NoError(t, client.Update(func(txn db.Transaction) error {
		for i := 0; i < count; i++ {
			space := &Space{
				ID:          db.NewDigest([]byte(fmt.Sprintf("space%d", i))),
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

func createTestThreads(t *testing.T, client *DatabaseClient, spaceID db.Digest, count int) []*Thread {
	threads := make([]*Thread, count)
	require.NoError(t, client.Update(func(txn db.Transaction) error {
		for i := 0; i < count; i++ {
			thread := &Thread{
				ID:        db.NewDigest([]byte(fmt.Sprintf("thread%d", i))),
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

func createTestMessages(t *testing.T, client *DatabaseClient, threadID db.Digest, count int) []*CompoundMessage {
	messages := make([]*CompoundMessage, count)
	require.NoError(t, client.Update(func(txn db.Transaction) error {
		for i := 0; i < count; i++ {
			message := &CompoundMessage{
				ID:        db.NewDigest([]byte(fmt.Sprintf("message%d", i))),
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

func createTestModels(t *testing.T, client *DatabaseClient, userID db.Digest, count int) []*ModelInfo {
	models := make([]*ModelInfo, count)
	require.NoError(t, client.Update(func(txn db.Transaction) error {
		for i := 0; i < count; i++ {
			model := &ModelInfo{
				ID:        db.NewDigest([]byte(fmt.Sprintf("model%d", i))),
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

func TestGetModelsByParent(t *testing.T) {
	dbManager, err := NewDatabaseManager(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, dbManager.Setup())
	defer dbManager.Close()

	// Create a parent model
	parentID := db.NewDigest([]byte("parent-model"))
	userID := db.NewDigest([]byte("test-user"))

	// Create parent model
	err = dbManager.Update(func(txn *DatabaseTransaction) error {
		parent := &ModelInfo{
			ID:             parentID,
			UserID:         userID,
			Name:           "llama-8b",
			ModelType:      ModelTypeBase,
			BaseModel:      "llama-8b",
			ModelSize:      "8B",
			Status:         ModelStatusReady,
			CheckpointPath: "/mnt/storage/llama-8b",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		return txn.SetModel(userID, parent)
	})
	require.NoError(t, err)

	// Create child models
	var childModels []*ModelInfo
	for i := 0; i < 5; i++ {
		childID := db.NewDigest([]byte(fmt.Sprintf("child-%d", i)))
		child := &ModelInfo{
			ID:             childID,
			UserID:         userID,
			Name:           fmt.Sprintf("llama-8b-u1-c%d", i+1),
			ModelType:      ModelTypeClone,
			BaseModel:      "llama-8b",
			ModelSize:      "8B",
			ParentID:       &parentID,
			Status:         ModelStatusReady,
			CheckpointPath: "/mnt/storage/llama-8b",
			CreatedAt:      time.Now().Add(time.Duration(i) * time.Minute),
			UpdatedAt:      time.Now().Add(time.Duration(i) * time.Minute),
		}
		childModels = append(childModels, child)

		err := dbManager.Update(func(txn *DatabaseTransaction) error {
			return txn.SetModel(userID, child)
		})
		require.NoError(t, err)
	}

	// Test retrieving children
	var result []*ModelInfo
	err = dbManager.View(func(txn *DatabaseTransaction) error {
		var err error
		result, err = txn.GetModelsByParent(parentID, 3, uint64(time.Now().Add(time.Hour).UnixNano()))
		return err
	})
	require.NoError(t, err)
	require.Len(t, result, 3)

	// Should get most recent 3 children in reverse order
	for i := 0; i < 3; i++ {
		require.Equal(t, childModels[4-i].ID, result[i].ID)
		require.Equal(t, childModels[4-i].Name, result[i].Name)
	}
}

func TestParentIndexDeletion(t *testing.T) {
	dbManager, err := NewDatabaseManager(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, dbManager.Setup())
	defer dbManager.Close()

	parentID := db.NewDigest([]byte("parent"))
	childID := db.NewDigest([]byte("child"))
	userID := db.NewDigest([]byte("user"))

	// Create parent and child
	err = dbManager.Update(func(txn *DatabaseTransaction) error {
		child := &ModelInfo{
			ID:             childID,
			UserID:         userID,
			Name:           "child-model",
			ModelType:      ModelTypeClone,
			ParentID:       &parentID,
			Status:         ModelStatusReady,
			CheckpointPath: "/path",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		return txn.SetModel(userID, child)
	})
	require.NoError(t, err)

	// Verify child exists in parent index
	var result []*ModelInfo
	err = dbManager.View(func(txn *DatabaseTransaction) error {
		var err error
		result, err = txn.GetModelsByParent(parentID, 10, uint64(time.Now().Add(time.Hour).UnixNano()))
		return err
	})
	require.NoError(t, err)
	require.Len(t, result, 1)

	// Delete model (which should also remove from parent index)
	err = dbManager.Update(func(txn *DatabaseTransaction) error {
		// In real usage, we'd have a DeleteModel method that handles the parent index
		// For now, let's just test the low-level deletion
		return txn.client.DeleteModelParent(txn.txn, parentID, result[0].UpdatedAt)
	})
	require.NoError(t, err)

	// Verify deletion
	err = dbManager.View(func(txn *DatabaseTransaction) error {
		var err error
		result, err = txn.GetModelsByParent(parentID, 10, uint64(time.Now().Add(time.Hour).UnixNano()))
		return err
	})
	require.NoError(t, err)
	require.Len(t, result, 0)
}

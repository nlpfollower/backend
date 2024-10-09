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

func TestGetUserThreadsReverse(t *testing.T) {
	client := NewDatabaseClient(t.TempDir())
	require.NoError(t, client.Setup())
	defer client.Close()

	userID := NewDigest([]byte("user1"))
	threads := createTestThreads(t, client, userID, 10)

	var result []*Thread
	require.NoError(t, client.View(func(txn db.Transaction) error {
		var err error
		result, err = client.GetUserThreadsReverse(txn, userID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 5)
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

func TestSecondaryIndexes(t *testing.T) {
	client := NewDatabaseClient(t.TempDir())
	require.NoError(t, client.Setup())
	defer client.Close()

	userID := NewDigest([]byte("user1"))
	threadID := NewDigest([]byte("thread1"))

	// Create test data
	threads := createTestThreads(t, client, userID, 5)
	messages := createTestMessages(t, client, threadID, 5)

	t.Run("GetUserThreadsReverse", func(t *testing.T) {
		var result []*Thread
		require.NoError(t, client.View(func(txn db.Transaction) error {
			var err error
			result, err = client.GetUserThreadsReverse(txn, userID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 3)
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

	t.Run("DeleteUserThread", func(t *testing.T) {
		var result []*Thread
		require.NoError(t, client.Update(func(txn db.Transaction) error {
			err := client.DeleteUserThread(txn, userID, threads[2].UpdatedAt)
			require.NoError(t, err)

			result, err = client.GetUserThreadsReverse(txn, userID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 5)
			return err
		}))
		require.Len(t, result, 4)
		require.NotEqual(t, threads[2].ID, result[2].ID)
	})

	t.Run("DeleteThreadMessage", func(t *testing.T) {
		var result []*CompoundMessage
		require.NoError(t, client.Update(func(txn db.Transaction) error {
			err := client.DeleteThreadMessage(txn, threadID, messages[2].CreatedAt)
			require.NoError(t, err)

			result, err = client.GetThreadMessagesReverse(txn, threadID.Bytes(), uint64(time.Now().Add(time.Hour).UnixNano()), 5)
			return err
		}))
		require.Len(t, result, 4)
		require.NotEqual(t, messages[2].ID, result[2].ID)
	})
}

// Helper functions

func createTestThreads(t *testing.T, client *DatabaseClient, userID Digest, count int) []*Thread {
	threads := make([]*Thread, count)
	require.NoError(t, client.Update(func(txn db.Transaction) error {
		for i := 0; i < count; i++ {
			thread := &Thread{
				ID:        NewDigest([]byte(fmt.Sprintf("thread%d", i))),
				UserID:    userID,
				Title:     fmt.Sprintf("Thread %d", i),
				CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
				UpdatedAt: time.Now().Add(time.Duration(i) * time.Minute),
			}
			err := client.SetThread(txn, thread)
			require.NoError(t, err)
			err = client.SetUserThread(txn, userID, thread.UpdatedAt, thread)
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

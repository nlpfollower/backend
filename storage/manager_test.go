package storage

import (
	"fmt"
	"testing"
	"time"

	"github.com/nlpfollower/deltamind/database/db"
	"github.com/stretchr/testify/require"
)

func TestGetMessagePath(t *testing.T) {
	dbManager, err := NewDatabaseManager(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, dbManager.Setup())
	defer dbManager.Close()

	t.Run("Basic Path", func(t *testing.T) {
		var messages []*CompoundMessage
		threadID := db.NewDigest([]byte("test-thread"))

		// Create a chain of messages
		require.NoError(t, dbManager.Update(func(txn *DatabaseTransaction) error {
			for i := 0; i < 5; i++ {
				msg := &CompoundMessage{
					ID:        db.NewDigest([]byte(fmt.Sprintf("msg-%d", i))),
					ThreadID:  threadID,
					Messages:  []string{fmt.Sprintf("Message %d", i)},
					Author:    "user",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}

				if i > 0 {
					msg.ParentID = &CompoundMessageID{
						ID:        messages[i-1].ID,
						MessageID: 0,
					}
				}

				err := txn.SetMessage(threadID, msg)
				require.NoError(t, err)
				messages = append(messages, msg)
				time.Sleep(time.Millisecond) // Ensure unique timestamps
			}
			return nil
		}))

		// Test fetching the entire chain
		require.NoError(t, dbManager.View(func(txn *DatabaseTransaction) error {
			path, err := txn.GetMessagePath(messages[4].ID, 0, 5)
			require.NoError(t, err)
			require.Len(t, path, 5)

			// Verify chronological order
			for i := 0; i < 5; i++ {
				require.Equal(t, messages[i].ID, path[i].ID)
				require.Equal(t, messages[i].Messages[0], path[i].Messages[0])
			}
			return nil
		}))
	})

	t.Run("Limited Path", func(t *testing.T) {
		var messages []*CompoundMessage
		threadID := db.NewDigest([]byte("test-thread-2"))

		// Create messages
		require.NoError(t, dbManager.Update(func(txn *DatabaseTransaction) error {
			for i := 0; i < 5; i++ {
				msg := &CompoundMessage{
					ID:        db.NewDigest([]byte(fmt.Sprintf("msg-2-%d", i))),
					ThreadID:  threadID,
					Messages:  []string{fmt.Sprintf("Message %d", i)},
					Author:    "user",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}

				if i > 0 {
					msg.ParentID = &CompoundMessageID{
						ID:        messages[i-1].ID,
						MessageID: 0,
					}
				}

				err := txn.SetMessage(threadID, msg)
				require.NoError(t, err)
				messages = append(messages, msg)
				time.Sleep(time.Millisecond)
			}
			return nil
		}))

		// Test fetching with limit
		require.NoError(t, dbManager.View(func(txn *DatabaseTransaction) error {
			path, err := txn.GetMessagePath(messages[4].ID, 0, 3)
			require.NoError(t, err)
			require.Len(t, path, 3)

			// Verify we got the most recent 3 messages in chronological order
			for i := 0; i < 3; i++ {
				require.Equal(t, messages[i+2].ID, path[i].ID)
			}
			return nil
		}))
	})

	t.Run("Invalid Message Index", func(t *testing.T) {
		threadID := db.NewDigest([]byte("test-thread-3"))

		var msgID db.Digest
		require.NoError(t, dbManager.Update(func(txn *DatabaseTransaction) error {
			msg := &CompoundMessage{
				ID:        db.NewDigest([]byte("msg-3")),
				ThreadID:  threadID,
				Messages:  []string{"Message"},
				Author:    "user",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			msgID = msg.ID
			return txn.SetMessage(threadID, msg)
		}))

		require.NoError(t, dbManager.View(func(txn *DatabaseTransaction) error {
			_, err := txn.GetMessagePath(msgID, 1, 5)
			require.Error(t, err)
			require.Contains(t, err.Error(), "invalid message index")
			return nil
		}))
	})

	t.Run("Non-existent Message", func(t *testing.T) {
		require.NoError(t, dbManager.View(func(txn *DatabaseTransaction) error {
			_, err := txn.GetMessagePath(db.NewDigest([]byte("non-existent")), 0, 5)
			require.Error(t, err)
			require.Contains(t, err.Error(), "message not found")
			return nil
		}))
	})

	t.Run("Invalid Limit", func(t *testing.T) {
		require.NoError(t, dbManager.View(func(txn *DatabaseTransaction) error {
			_, err := txn.GetMessagePath(db.NewDigest([]byte("any")), 0, 0)
			require.Error(t, err)
			require.Contains(t, err.Error(), "limit must be positive")
			return nil
		}))
	})
}

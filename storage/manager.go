package storage

import (
	"fmt"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
)

type DatabaseManager struct {
	client *DatabaseClient
}

func NewDatabaseManager(dbPath string) (*DatabaseManager, error) {
	// Create the directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}
	client := NewDatabaseClient(dbPath)
	return &DatabaseManager{
		client: client,
	}, nil
}

func (manager *DatabaseManager) Setup() error {
	return manager.client.Setup()
}

func (manager *DatabaseManager) Close() error {
	return manager.client.Close()
}

func (manager *DatabaseManager) GetDBPath() string {
	return manager.client.GetDBPath()
}

func (manager *DatabaseManager) CreateUser(user *User) error {
	if user == nil {
		return errors.New("SetUser: user is nil")
	}

	return manager.client.Update(func(txn db.Transaction) error {
		return manager.client.SetUser(txn, user)
	})
}

func (manager *DatabaseManager) GetUser(userID Digest) (*User, error) {
	var user *User
	err := manager.client.View(func(txn db.Transaction) error {
		var err error
		user, err = manager.client.GetUser(txn, userID)
		return err
	})
	return user, err
}

func (manager *DatabaseManager) DeleteUser(userID Digest) error {

	return manager.client.Update(func(txn db.Transaction) error {
		// First, get the user to ensure it exists
		user, err := manager.client.GetUser(txn, userID)
		if err != nil {
			return errors.Wrap(err, "DeleteUser: failed to get user")
		}
		if user == nil {
			return fmt.Errorf("DeleteUser: user not found for ID: %s", userID)
		}

		// Delete the user
		err = manager.client.DeleteUser(txn, userID)
		if err != nil {
			return errors.Wrap(err, "DeleteUser: failed to delete user")
		}

		// TODO: Consider if deleting associated data (e.g., threads, messages)

		return nil
	})
}

// This shouldn't be called for updating a thread.
func (manager *DatabaseManager) CreateThread(userID Digest, thread *Thread) error {
	if thread == nil {
		return errors.New("CreateThread: thread is nil")
	}

	return manager.client.Update(func(txn db.Transaction) error {
		err := manager.client.SetThread(txn, thread)
		if err != nil {
			return errors.Wrap(err, "CreateThread: failed to set thread")
		}

		err = manager.client.SetUserThread(txn, userID, thread.UpdatedAt, thread)
		if err != nil {
			return errors.Wrap(err, "CreateThread: failed to set user thread")
		}

		return nil
	})
}

func (manager *DatabaseManager) GetThread(threadID Digest) (*Thread, error) {
	var thread *Thread
	err := manager.client.View(func(txn db.Transaction) error {
		var err error
		thread, err = manager.client.GetThread(txn, threadID)
		return err
	})
	return thread, err
}

func (manager *DatabaseManager) GetUserThreads(userID Digest, limit int, maxTimestamp uint64) ([]*Thread, error) {
	var threads []*Thread

	err := manager.client.View(func(txn db.Transaction) error {
		var err error
		threads, err = manager.client.GetUserThreadsReverse(txn, userID.Bytes(), maxTimestamp, limit)
		return err
	})

	return threads, err
}

func (manager *DatabaseManager) CreateMessage(threadID Digest, msg *CompoundMessage) error {
	if msg == nil {
		return errors.New("CreateMessage: message is nil")
	}

	return manager.client.Update(func(txn db.Transaction) error {
		err := manager.client.SetMessage(txn, msg)
		if err != nil {
			return errors.Wrap(err, "CreateMessage: failed to set message")
		}

		err = manager.client.SetThreadMessage(txn, threadID, msg.UpdatedAt, msg)
		if err != nil {
			return errors.Wrap(err, "CreateMessage: failed to set thread message")
		}

		return nil
	})
}

func (manager *DatabaseManager) GetMessage(messageID Digest) (*CompoundMessage, error) {
	var message *CompoundMessage
	err := manager.client.View(func(txn db.Transaction) error {
		var err error
		message, err = manager.client.GetMessage(txn, messageID)
		return err
	})
	return message, err
}

func (manager *DatabaseManager) GetThreadMessages(threadID Digest, limit int, maxTimestamp uint64) ([]*CompoundMessage, error) {
	var messages []*CompoundMessage
	err := manager.client.View(func(txn db.Transaction) error {
		var err error
		messages, err = manager.client.GetThreadMessagesReverse(txn, threadID.Bytes(), maxTimestamp, limit)
		return err
	})
	return messages, err
}

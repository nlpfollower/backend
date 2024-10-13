package storage

import (
	"fmt"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/pkg/errors"
	"math"
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

// ==========================
// User operations
// ==========================
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

// ==========================
// Space operations
// ==========================
func (manager *DatabaseManager) CreateSpace(userID Digest, space *Space) error {
	if space == nil {
		return errors.New("CreateSpace: space is nil")
	}

	return manager.client.Update(func(txn db.Transaction) error {
		err := manager.client.SetSpace(txn, space)
		if err != nil {
			return errors.Wrap(err, "CreateSpace: failed to set space")
		}

		err = manager.client.SetUserSpace(txn, userID, space.UpdatedAt, space)
		if err != nil {
			return errors.Wrap(err, "CreateSpace: failed to set user space")
		}

		return nil
	})
}

func (manager *DatabaseManager) GetSpace(spaceID Digest) (*Space, error) {
	var space *Space
	err := manager.client.View(func(txn db.Transaction) error {
		var err error
		space, err = manager.client.GetSpace(txn, spaceID)
		return err
	})
	return space, err
}

func (manager *DatabaseManager) GetUserSpaces(userID Digest, limit int, maxTimestamp uint64) ([]*Space, error) {
	var spaces []*Space

	err := manager.client.View(func(txn db.Transaction) error {
		var err error
		spaces, err = manager.client.GetUserSpacesReverse(txn, userID.Bytes(), maxTimestamp, limit)
		return err
	})

	return spaces, err
}

func (manager *DatabaseManager) DeleteSpace(spaceID Digest) error {
	return manager.client.Update(func(txn db.Transaction) error {
		space, err := manager.client.GetSpace(txn, spaceID)
		if err != nil {
			return errors.Wrap(err, "DeleteSpace: failed to get space")
		}
		if space == nil {
			return fmt.Errorf("DeleteSpace: space not found for ID: %s", spaceID)
		}

		err = manager.client.DeleteSpace(txn, spaceID)
		if err != nil {
			return errors.Wrap(err, "DeleteSpace: failed to delete space")
		}

		err = manager.client.DeleteUserSpace(txn, space.UserID, space.UpdatedAt)
		if err != nil {
			return errors.Wrap(err, "DeleteSpace: failed to delete user space")
		}

		// TODO: Consider deleting associated threads and messages

		return nil
	})
}

// ==========================
// Thread operations
// ==========================
// This shouldn't be called for updating a thread.
func (manager *DatabaseManager) CreateThread(spaceID Digest, thread *Thread) error {
	if thread == nil {
		return errors.New("CreateThread: thread is nil")
	}

	return manager.client.Update(func(txn db.Transaction) error {
		err := manager.client.SetThread(txn, thread)
		if err != nil {
			return errors.Wrap(err, "CreateThread: failed to set thread")
		}

		err = manager.client.SetSpaceThread(txn, spaceID, thread.UpdatedAt, thread)
		if err != nil {
			return errors.Wrap(err, "CreateThread: failed to set space thread")
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

func (manager *DatabaseManager) GetSpaceThreads(spaceID Digest, limit int, maxTimestamp uint64) ([]*Thread, error) {
	var threads []*Thread

	err := manager.client.View(func(txn db.Transaction) error {
		var err error
		threads, err = manager.client.GetSpaceThreadsReverse(txn, spaceID.Bytes(), maxTimestamp, limit)
		return err
	})

	return threads, err
}

func (manager *DatabaseManager) DeleteThread(threadID Digest) error {
	return manager.client.Update(func(txn db.Transaction) error {
		thread, err := manager.client.GetThread(txn, threadID)
		if err != nil {
			return errors.Wrap(err, "DeleteThread: failed to get thread")
		}
		if thread == nil {
			return fmt.Errorf("DeleteThread: thread not found for ID: %s", threadID)
		}

		err = manager.client.DeleteThread(txn, threadID)
		if err != nil {
			return errors.Wrap(err, "DeleteThread: failed to delete thread")
		}

		err = manager.client.DeleteSpaceThread(txn, thread.SpaceID, thread.UpdatedAt)
		if err != nil {
			return errors.Wrap(err, "DeleteThread: failed to delete space thread")
		}

		// TODO: Consider deleting associated messages

		return nil
	})
}

// ==========================
// Message operations
// ==========================
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

func (manager *DatabaseManager) DeleteMessage(messageID Digest) error {
	return manager.client.Update(func(txn db.Transaction) error {
		message, err := manager.client.GetMessage(txn, messageID)
		if err != nil {
			return errors.Wrap(err, "DeleteMessage: failed to get message")
		}
		if message == nil {
			return fmt.Errorf("DeleteMessage: message not found for ID: %s", messageID)
		}

		err = manager.client.DeleteMessage(txn, messageID)
		if err != nil {
			return errors.Wrap(err, "DeleteMessage: failed to delete message")
		}

		err = manager.client.DeleteThreadMessage(txn, message.ThreadID, message.UpdatedAt)
		if err != nil {
			return errors.Wrap(err, "DeleteMessage: failed to delete thread message")
		}

		return nil
	})
}

// ==========================
// Model operations
// ==========================
func (manager *DatabaseManager) CreateModel(userID Digest, model *ModelInfo) error {
	if model == nil {
		return errors.New("CreateModel: model is nil")
	}

	return manager.client.Update(func(txn db.Transaction) error {
		err := manager.client.SetModel(txn, model)
		if err != nil {
			return errors.Wrap(err, "CreateModel: failed to set model")
		}

		err = manager.client.SetUserModel(txn, userID, model.UpdatedAt, model)
		if err != nil {
			return errors.Wrap(err, "CreateModel: failed to set user model")
		}

		return nil
	})
}

func (manager *DatabaseManager) GetModel(modelID Digest) (*ModelInfo, error) {
	var model *ModelInfo
	err := manager.client.View(func(txn db.Transaction) error {
		var err error
		model, err = manager.client.GetModel(txn, modelID)
		return err
	})
	return model, err
}

func (manager *DatabaseManager) GetUserModels(userID Digest, limit int, maxTimestamp uint64) ([]*ModelInfo, error) {
	var models []*ModelInfo

	err := manager.client.View(func(txn db.Transaction) error {
		var err error
		models, err = manager.client.GetUserModelsReverse(txn, userID.Bytes(), maxTimestamp, limit)
		return err
	})

	return models, err
}

func (manager *DatabaseManager) DeleteModel(modelID Digest) error {
	return manager.client.Update(func(txn db.Transaction) error {
		model, err := manager.client.GetModel(txn, modelID)
		if err != nil {
			return errors.Wrap(err, "DeleteModel: failed to get model")
		}
		if model == nil {
			return fmt.Errorf("DeleteModel: model not found for ID: %s", modelID)
		}

		err = manager.client.DeleteModel(txn, modelID)
		if err != nil {
			return errors.Wrap(err, "DeleteModel: failed to delete model")
		}

		err = manager.client.DeleteUserModel(txn, model.UserID, model.UpdatedAt)
		if err != nil {
			return errors.Wrap(err, "DeleteModel: failed to delete user model")
		}

		// TODO: Consider deleting associated model iterations

		return nil
	})
}

// ==========================
// ModelIteration operations
// ==========================
func (manager *DatabaseManager) CreateModelIteration(modelID Digest, iteration *ModelIteration) error {
	if iteration == nil {
		return errors.New("CreateModelIteration: iteration is nil")
	}

	return manager.client.Update(func(txn db.Transaction) error {
		// Ensure the previous iteration exists, unless it's the first iteration
		if iteration.Index > 0 {
			// Get the latest iteration to determine the next index
			latestIterations, err := manager.client.GetModelIterationsReverse(txn, modelID.Bytes(), math.MaxUint64, 1)
			if err != nil {
				return errors.Wrap(err, "CreateModelIteration: failed to get latest iteration")
			}
			if len(latestIterations) == 0 {
				return fmt.Errorf("CreateModelIteration: previous iteration (index %d) does not exist", iteration.Index-1)
			}
		}

		err := manager.client.SetModelIteration(txn, modelID, iteration.Index, iteration)
		if err != nil {
			return errors.Wrap(err, "CreateModelIteration: failed to set model iteration")
		}

		return nil
	})
}

func (manager *DatabaseManager) GetModelIterations(modelID Digest, maxIndex, limit uint64) ([]*ModelIteration, error) {
	var iterations []*ModelIteration

	err := manager.client.View(func(txn db.Transaction) error {
		var err error
		iterations, err = manager.client.GetModelIterationsReverse(txn, modelID.Bytes(), maxIndex, limit)
		return err
	})

	return iterations, err
}

func (manager *DatabaseManager) DeleteModelIteration(modelID Digest, index uint64) error {
	return manager.client.Update(func(txn db.Transaction) error {
		iterations, err := manager.client.GetModelIterationsReverse(txn, modelID.Bytes(), index, 1)
		if err != nil {
			return errors.Wrap(err, "DeleteModelIteration: failed to get iteration")
		}
		if len(iterations) == 0 {
			return fmt.Errorf("DeleteModelIteration: iteration not found for model ID: %s and index: %d", modelID, index)
		}

		err = manager.client.DeleteModelIteration(txn, modelID, index)
		if err != nil {
			return errors.Wrap(err, "DeleteModelIteration: failed to delete model iteration")
		}

		return nil
	})
}

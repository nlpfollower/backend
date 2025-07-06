package storage

import (
	"fmt"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
	"time"
)

// TransactionFunc represents a function that runs within a transaction
type TransactionFunc func(m *DatabaseTransaction) error

type DatabaseTransaction struct {
	client *DatabaseClient
	txn    db.Transaction
}

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

func (manager *DatabaseManager) Update(fn TransactionFunc) error {
	return manager.client.Update(func(txn db.Transaction) error {
		dbTxn := &DatabaseTransaction{
			client: manager.client,
			txn:    txn,
		}

		return fn(dbTxn)
	})
}

func (manager *DatabaseManager) View(fn TransactionFunc) error {
	return manager.client.View(func(txn db.Transaction) error {
		dbTxn := &DatabaseTransaction{
			client: manager.client,
			txn:    txn,
		}
		return fn(dbTxn)
	})
}

// ==========================
// User operations
// ==========================
func (dbTxn *DatabaseTransaction) SetUser(user *User) error {
	if user == nil {
		return errors.New("SetUser: user is nil")
	}

	return dbTxn.client.SetUser(dbTxn.txn, user)
}

func (dbTxn *DatabaseTransaction) GetUser(userID db.Digest) (*User, error) {
	return dbTxn.client.GetUser(dbTxn.txn, userID)
}

func (dbTxn *DatabaseTransaction) DeleteUser(userID db.Digest) error {
	// First, get the user to ensure it exists
	user, err := dbTxn.client.GetUser(dbTxn.txn, userID)
	if err != nil {
		return errors.Wrap(err, "DeleteUser: failed to get user")
	}
	if user == nil {
		return nil
	}

	// Delete the user
	err = dbTxn.client.DeleteUser(dbTxn.txn, userID)
	if err != nil {
		return errors.Wrap(err, "DeleteUser: failed to delete user")
	}

	// TODO: Consider if deleting associated data (e.g., threads, messages)

	return nil
}

// ==========================
// Space operations
// ==========================
func (dbTxn *DatabaseTransaction) SetSpace(userID db.Digest, space *Space) error {
	if space == nil {
		return errors.New("SetSpace: space is nil")
	}

	// Check if this is an update by looking for existing space
	existingSpace, _ := dbTxn.client.GetSpace(dbTxn.txn, space.ID)

	err := dbTxn.client.SetSpace(dbTxn.txn, space)
	if err != nil {
		return errors.Wrap(err, "SetSpace: failed to set space")
	}

	// If updating, remove old secondary index entry
	if existingSpace != nil && existingSpace.UpdatedAt != space.UpdatedAt {
		err = dbTxn.client.DeleteUserSpace(dbTxn.txn, userID, existingSpace.UpdatedAt)
		if err != nil {
			return errors.Wrap(err, "SetSpace: failed to delete old user space index")
		}
	}

	err = dbTxn.client.SetUserSpace(dbTxn.txn, userID, space.UpdatedAt, space)
	if err != nil {
		return errors.Wrap(err, "SetSpace: failed to set user space")
	}

	return nil
}

func (dbTxn *DatabaseTransaction) GetSpace(spaceID db.Digest) (*Space, error) {
	return dbTxn.client.GetSpace(dbTxn.txn, spaceID)
}

func (dbTxn *DatabaseTransaction) GetUserSpaces(userID db.Digest, limit int, maxTimestamp uint64) ([]*Space, error) {
	return dbTxn.client.GetUserSpacesReverse(dbTxn.txn, userID.Bytes(), maxTimestamp, limit)
}

func (dbTxn *DatabaseTransaction) DeleteSpace(spaceID db.Digest) error {
	space, err := dbTxn.client.GetSpace(dbTxn.txn, spaceID)
	if err != nil {
		return errors.Wrap(err, "DeleteSpace: failed to get space")
	}
	if space == nil {
		return nil
	}

	err = dbTxn.client.DeleteSpace(dbTxn.txn, spaceID)
	if err != nil {
		return errors.Wrap(err, "DeleteSpace: failed to delete space")
	}

	err = dbTxn.client.DeleteUserSpace(dbTxn.txn, space.UserID, space.UpdatedAt)
	if err != nil {
		return errors.Wrap(err, "DeleteSpace: failed to delete user space")
	}

	return nil
}

// ==========================
// Thread operations
// ==========================
func (dbTxn *DatabaseTransaction) SetThread(spaceID db.Digest, thread *Thread) error {
	if thread == nil {
		return errors.New("SetThread: thread is nil")
	}

	// Check if this is an update by looking for existing thread
	existingThread, _ := dbTxn.client.GetThread(dbTxn.txn, thread.ID)

	err := dbTxn.client.SetThread(dbTxn.txn, thread)
	if err != nil {
		return errors.Wrap(err, "SetThread: failed to set thread")
	}

	// If updating, remove old secondary index entry
	if existingThread != nil && existingThread.UpdatedAt != thread.UpdatedAt {
		err = dbTxn.client.DeleteSpaceThread(dbTxn.txn, spaceID, existingThread.UpdatedAt)
		if err != nil {
			return errors.Wrap(err, "SetThread: failed to delete old space thread index")
		}
	}

	err = dbTxn.client.SetSpaceThread(dbTxn.txn, spaceID, thread.UpdatedAt, thread)
	if err != nil {
		return errors.Wrap(err, "SetThread: failed to set space thread")
	}

	return nil
}

func (dbTxn *DatabaseTransaction) GetThread(threadID db.Digest) (*Thread, error) {
	return dbTxn.client.GetThread(dbTxn.txn, threadID)
}

func (dbTxn *DatabaseTransaction) GetSpaceThreads(spaceID db.Digest, limit int, maxTimestamp uint64) ([]*Thread, error) {
	return dbTxn.client.GetSpaceThreadsReverse(dbTxn.txn, spaceID.Bytes(), maxTimestamp, limit)
}

func (dbTxn *DatabaseTransaction) DeleteThread(threadID db.Digest) error {
	thread, err := dbTxn.client.GetThread(dbTxn.txn, threadID)
	if err != nil {
		return errors.Wrap(err, "DeleteThread: failed to get thread")
	}
	if thread == nil {
		return nil
	}

	err = dbTxn.client.DeleteThread(dbTxn.txn, threadID)
	if err != nil {
		return errors.Wrap(err, "DeleteThread: failed to delete thread")
	}

	err = dbTxn.client.DeleteSpaceThread(dbTxn.txn, thread.SpaceID, thread.UpdatedAt)
	if err != nil {
		return errors.Wrap(err, "DeleteThread: failed to delete space thread")
	}

	return nil
}

// ==========================
// Message operations
// ==========================
func (dbTxn *DatabaseTransaction) SetMessage(threadID db.Digest, msg *CompoundMessage) error {
	if msg == nil {
		return errors.New("SetMessage: message is nil")
	}

	// Check if this is an update by looking for existing message
	existingMsg, _ := dbTxn.client.GetMessage(dbTxn.txn, msg.ID)

	err := dbTxn.client.SetMessage(dbTxn.txn, msg)
	if err != nil {
		return errors.Wrap(err, "SetMessage: failed to set message")
	}

	// If updating, remove old secondary index entry
	if existingMsg != nil && existingMsg.UpdatedAt != msg.UpdatedAt {
		err = dbTxn.client.DeleteThreadMessage(dbTxn.txn, threadID, existingMsg.UpdatedAt)
		if err != nil {
			return errors.Wrap(err, "SetMessage: failed to delete old thread message index")
		}
	}

	err = dbTxn.client.SetThreadMessage(dbTxn.txn, threadID, msg.UpdatedAt, msg)
	if err != nil {
		return errors.Wrap(err, "SetMessage: failed to set thread message")
	}

	return nil
}

func (dbTxn *DatabaseTransaction) GetMessage(messageID db.Digest) (*CompoundMessage, error) {
	return dbTxn.client.GetMessage(dbTxn.txn, messageID)
}

func (dbTxn *DatabaseTransaction) GetThreadMessages(threadID db.Digest, limit int, maxTimestamp uint64) ([]*CompoundMessage, error) {
	return dbTxn.client.GetThreadMessagesReverse(dbTxn.txn, threadID.Bytes(), maxTimestamp, limit)
}

func (dbTxn *DatabaseTransaction) DeleteMessage(messageID db.Digest) error {
	message, err := dbTxn.client.GetMessage(dbTxn.txn, messageID)
	if err != nil {
		return errors.Wrap(err, "DeleteMessage: failed to get message")
	}
	if message == nil {
		return nil
	}

	err = dbTxn.client.DeleteMessage(dbTxn.txn, messageID)
	if err != nil {
		return errors.Wrap(err, "DeleteMessage: failed to delete message")
	}

	err = dbTxn.client.DeleteThreadMessage(dbTxn.txn, message.ThreadID, message.UpdatedAt)
	if err != nil {
		return errors.Wrap(err, "DeleteMessage: failed to delete thread message")
	}

	return nil
}

func (dbTxn *DatabaseTransaction) GetMessagePath(leafMessageID db.Digest, messageIndex uint64, limit int) ([]*CompoundMessage, error) {
	if limit <= 0 {
		return nil, errors.New("GetMessagePath: limit must be positive")
	}

	// Get the initial message
	leafMessage, err := dbTxn.GetMessage(leafMessageID)
	if err != nil {
		return nil, errors.Wrap(err, "GetMessagePath: failed to get leaf message")
	}
	if leafMessage == nil {
		return nil, errors.New("GetMessagePath: leaf message not found")
	}

	// Validate message index
	if int(messageIndex) >= len(leafMessage.Messages) {
		return nil, fmt.Errorf("GetMessagePath: invalid message index %d for message with %d messages",
			messageIndex, len(leafMessage.Messages))
	}

	var messages []*CompoundMessage
	messages = append(messages, leafMessage)
	currentMsg := leafMessage

	// Follow the parent chain until we reach the limit or a message without a parent
	for len(messages) < limit && currentMsg.ParentID != nil {
		parentMsg, err := dbTxn.GetMessage(currentMsg.ParentID.ID)
		if err != nil {
			return nil, errors.Wrap(err, "GetMessagePath: failed to get parent message")
		}
		if parentMsg == nil {
			break // Parent message not found - could have been deleted
		}

		// Validate parent message index
		if int(currentMsg.ParentID.MessageID) >= len(parentMsg.Messages) {
			return nil, fmt.Errorf("GetMessagePath: invalid parent message index %d for message with %d messages",
				currentMsg.ParentID.MessageID, len(parentMsg.Messages))
		}

		messages = append(messages, parentMsg)
		currentMsg = parentMsg
	}

	// Reverse the slice to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// ==========================
// Model operations
// ==========================
func (dbTxn *DatabaseTransaction) SetModel(userID db.Digest, model *ModelInfo) error {
	if model == nil {
		return errors.New("SetModel: model is nil")
	}

	// Check if this is an update by looking for existing model
	existingModel, _ := dbTxn.client.GetModel(dbTxn.txn, model.ID)

	err := dbTxn.client.SetModel(dbTxn.txn, model)
	if err != nil {
		return errors.Wrap(err, "SetModel: failed to set model")
	}

	// If updating, remove old secondary index entries
	if existingModel != nil && existingModel.UpdatedAt != model.UpdatedAt {
		// Remove old user-model index
		err = dbTxn.client.DeleteUserModel(dbTxn.txn, userID, existingModel.UpdatedAt)
		if err != nil {
			return errors.Wrap(err, "SetModel: failed to delete old user model index")
		}

		// Remove old parent index if it had a parent
		if existingModel.ParentID != nil {
			err = dbTxn.client.DeleteModelParent(dbTxn.txn, *existingModel.ParentID, existingModel.UpdatedAt)
			if err != nil {
				return errors.Wrap(err, "SetModel: failed to delete old model parent index")
			}
		}
	}

	// Add new secondary index entries
	err = dbTxn.client.SetUserModel(dbTxn.txn, userID, model.UpdatedAt, model)
	if err != nil {
		return errors.Wrap(err, "SetModel: failed to set user model")
	}

	// If model has a parent, add to parent index
	if model.ParentID != nil {
		err = dbTxn.client.SetModelParent(dbTxn.txn, *model.ParentID, model.UpdatedAt, model)
		if err != nil {
			return errors.Wrap(err, "SetModel: failed to set model parent index")
		}
	}

	return nil
}

func (dbTxn *DatabaseTransaction) GetModel(modelID db.Digest) (*ModelInfo, error) {
	return dbTxn.client.GetModel(dbTxn.txn, modelID)
}

func (dbTxn *DatabaseTransaction) GetUserModels(userID db.Digest, limit int, maxTimestamp uint64) ([]*ModelInfo, error) {
	return dbTxn.client.GetUserModelsReverse(dbTxn.txn, userID.Bytes(), maxTimestamp, limit)
}

func (dbTxn *DatabaseTransaction) DeleteModel(modelID db.Digest) error {
	model, err := dbTxn.client.GetModel(dbTxn.txn, modelID)
	if err != nil {
		return errors.Wrap(err, "DeleteModel: failed to get model")
	}
	if model == nil {
		return nil
	}

	err = dbTxn.client.DeleteModel(dbTxn.txn, modelID)
	if err != nil {
		return errors.Wrap(err, "DeleteModel: failed to delete model")
	}

	err = dbTxn.client.DeleteUserModel(dbTxn.txn, model.UserID, model.UpdatedAt)
	if err != nil {
		return errors.Wrap(err, "DeleteModel: failed to delete user model")
	}

	// If model had a parent, remove from parent index
	if model.ParentID != nil {
		err = dbTxn.client.DeleteModelParent(dbTxn.txn, *model.ParentID, model.UpdatedAt)
		if err != nil {
			return errors.Wrap(err, "DeleteModel: failed to delete model parent index")
		}
	}

	return nil
}

func (dbTxn *DatabaseTransaction) GetModelsByParent(parentID db.Digest, limit int, maxTimestamp uint64) ([]*ModelInfo, error) {
	return dbTxn.client.GetModelsByParentReverse(dbTxn.txn, parentID.Bytes(), maxTimestamp, limit)
}

func (dbTxn *DatabaseTransaction) GetUserAndIncrementCloneNum(userID db.Digest) (*User, uint64, error) {
	user, err := dbTxn.client.GetUser(dbTxn.txn, userID)
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to get user")
	}
	if user == nil {
		return nil, 0, errors.New("user not found")
	}

	cloneNum := user.NextCloneNum
	user.NextCloneNum++

	if err := dbTxn.client.SetUser(dbTxn.txn, user); err != nil {
		return nil, 0, errors.Wrap(err, "failed to update user")
	}

	return user, cloneNum, nil
}

// CleanupDuplicateModels removes all duplicate model entries for a user
func (dbTxn *DatabaseTransaction) CleanupDuplicateModels(userID db.Digest) (int, error) {
	// Get all models for this user
	models, err := dbTxn.GetUserModels(userID, 10000, uint64(time.Now().UnixNano()))
	if err != nil {
		return 0, errors.Wrap(err, "failed to get user models")
	}

	// Group by model ID to find duplicates
	modelsByID := make(map[string][]*ModelInfo)
	for _, model := range models {
		idStr := model.ID.String()
		modelsByID[idStr] = append(modelsByID[idStr], model)
	}

	// Keep track of unique models (latest version of each)
	uniqueModels := make(map[string]*ModelInfo)
	duplicatesRemoved := 0

	// First pass: delete all secondary index entries
	for modelID, modelList := range modelsByID {
		// Find the latest version
		var latest *ModelInfo
		for _, model := range modelList {
			if latest == nil || model.UpdatedAt.After(latest.UpdatedAt) {
				latest = model
			}
		}
		uniqueModels[modelID] = latest

		// Delete ALL secondary index entries for this model
		for _, model := range modelList {
			// Delete from user-model index
			err := dbTxn.client.DeleteUserModel(dbTxn.txn, userID, model.UpdatedAt)
			if err != nil {
				// Log but continue
				fmt.Printf("Warning: failed to delete user-model index for %s at %s: %v\n",
					model.Name, model.UpdatedAt, err)
			}

			// Delete from parent index if exists
			if model.ParentID != nil {
				err := dbTxn.client.DeleteModelParent(dbTxn.txn, *model.ParentID, model.UpdatedAt)
				if err != nil {
					// Log but continue
					fmt.Printf("Warning: failed to delete parent index for %s: %v\n", model.Name, err)
				}
			}
		}

		if len(modelList) > 1 {
			duplicatesRemoved += len(modelList) - 1
		}
	}

	// Second pass: re-add the unique models with current timestamp
	for _, model := range uniqueModels {
		// Update timestamp to now for clean state
		model.UpdatedAt = time.Now()

		// Re-add to secondary indices
		err = dbTxn.client.SetUserModel(dbTxn.txn, userID, model.UpdatedAt, model)
		if err != nil {
			return duplicatesRemoved, errors.Wrap(err, "failed to re-add user model index")
		}

		if model.ParentID != nil {
			err = dbTxn.client.SetModelParent(dbTxn.txn, *model.ParentID, model.UpdatedAt, model)
			if err != nil {
				return duplicatesRemoved, errors.Wrap(err, "failed to re-add parent model index")
			}
		}
	}

	return duplicatesRemoved, nil
}

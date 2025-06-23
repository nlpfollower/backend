package storage

import (
	"encoding/binary"
	"encoding/json"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/pkg/errors"
	"time"
)

const (
	// Top-level contexts
	// DBPrimaryContext is used to store primary data
	DBPrimaryContext byte = 0x00
	// DBSecondaryContext is used to store secondary indices, derived from primary data
	DBSecondaryContext byte = 0x01

	// Prefixes for primary data
	// DBUserPrefix is used to store user data
	// <DBPrimaryContext><DBUserPrefix><UserID> -> <User>
	DBPrimaryUserPrefix byte = 0x00
	// DBSpacePrefix is used to store spaces data
	// <DBPrimaryContext><DBSpacePrefix><SpaceID> -> <Space>
	DBPrimarySpacePrefix byte = 0x01
	// DBThreadPrefix is used to store thread data
	// <DBPrimaryContext><DBThreadPrefix><ThreadID> -> <Thread>
	DBPrimaryThreadPrefix byte = 0x02
	// DBMessagePrefix is used to store message data
	// <DBPrimaryContext><DBMessagePrefix><MessageID> -> <Message>
	DBPrimaryMessagePrefix byte = 0x03
	// DBModelPrefix is used to store model information data
	// <DBPrimaryContext><DBModelPrefix><ModelID> -> <ModelInfo>
	DBPrimaryModelPrefix byte = 0x04

	// Prefixes for secondary indices
	// DBSecondaryUserSpacePrefix is used to store user's spaces
	// <DBSecondaryContext><DBSecondaryUserSpacePrefix><UserID><Timestamp> -> <Space>
	DBSecondaryUserSpacePrefix byte = 0x00
	// DBSecondarySpaceThreadPrefix is used to store space's threads
	// <DBSecondaryContext><DBSecondarySpaceThreadPrefix><SpaceID><Timestamp> -> <Thread>
	DBSecondarySpaceThreadPrefix byte = 0x01
	// DBSecondaryThreadMessageTimestampPrefix is used to store thread's messages by timestamp
	// <DBSecondaryContext><DBSecondaryThreadMessageTimestampPrefix><ThreadID><Timestamp> -> <CompositeMessage>
	DBSecondaryThreadMessageTimestampPrefix byte = 0x02
	// DBSecondaryUserModelPrefix is used to store user's models
	// <DBSecondaryContext><DBSecondaryUserModelPrefix><UserID><Timestamp> -> <ModelInfo>
	DBSecondaryUserModelPrefix byte = 0x03
	// DBSecondaryModelParentPrefix is used to store models by parent
	// <DBSecondaryContext><DBSecondaryModelParentPrefix><ParentID><Timestamp> -> <ModelInfo>
	DBSecondaryModelParentPrefix byte = 0x04
)

type DatabaseClient struct {
	*db.ProtectedDatabase
	dbPath string
}

func NewDatabaseClient(dbPath string) *DatabaseClient {
	boltDB := db.NewBoltDatabase(dbPath)
	protectedDB := db.NewProtectedDatabase(boltDB)
	return &DatabaseClient{
		ProtectedDatabase: protectedDB,
		dbPath:            dbPath,
	}
}

func (client *DatabaseClient) GetDBPath() string {
	return client.dbPath
}

// ==========================
// Key Getters
// ==========================
func (client *DatabaseClient) getKeyForPrimaryUser(userID []byte) (db.Context, []byte) {
	ctx := client.GetContext([]byte{DBPrimaryContext, DBPrimaryUserPrefix})
	return ctx, userID
}

func (client *DatabaseClient) getKeyForPrimarySpace(spaceID []byte) (db.Context, []byte) {
	ctx := client.GetContext([]byte{DBPrimaryContext, DBPrimarySpacePrefix})
	return ctx, spaceID
}

func (client *DatabaseClient) getKeyForPrimaryThread(threadID []byte) (db.Context, []byte) {
	ctx := client.GetContext([]byte{DBPrimaryContext, DBPrimaryThreadPrefix})
	return ctx, threadID
}

func (client *DatabaseClient) getKeyForPrimaryMessage(messageID []byte) (db.Context, []byte) {
	ctx := client.GetContext([]byte{DBPrimaryContext, DBPrimaryMessagePrefix})
	return ctx, messageID
}

func (client *DatabaseClient) getKeyForPrimaryModel(modelID []byte) (db.Context, []byte) {
	ctx := client.GetContext([]byte{DBPrimaryContext, DBPrimaryModelPrefix})
	return ctx, modelID
}

func (client *DatabaseClient) getKeyForSecondaryUserSpace(userID []byte, timestamp time.Time) (db.Context, []byte) {
	ctx := client.GetContext([]byte{DBSecondaryContext, DBSecondaryUserSpacePrefix})
	ts := NewTimestamp(timestamp)
	key := append(userID, ts[:]...)
	return ctx, key
}

func (client *DatabaseClient) getKeyForSecondarySpaceThread(spaceID []byte, timestamp time.Time) (db.Context, []byte) {
	ctx := client.GetContext([]byte{DBSecondaryContext, DBSecondarySpaceThreadPrefix})
	ts := NewTimestamp(timestamp)
	key := append(spaceID, ts[:]...)
	return ctx, key
}

func (client *DatabaseClient) getKeyForSecondaryThreadMessageTimestamp(threadID []byte, timestamp time.Time) (db.Context, []byte) {
	ctx := client.GetContext([]byte{DBSecondaryContext, DBSecondaryThreadMessageTimestampPrefix})
	ts := NewTimestamp(timestamp)
	key := append(threadID, ts[:]...)
	return ctx, key
}

func (client *DatabaseClient) getKeyForSecondaryUserModel(userID []byte, timestamp time.Time) (db.Context, []byte) {
	ctx := client.GetContext([]byte{DBSecondaryContext, DBSecondaryUserModelPrefix})
	ts := NewTimestamp(timestamp)
	key := append(userID, ts[:]...)
	return ctx, key
}

func (client *DatabaseClient) getKeyForSecondaryModelParent(parentID []byte, timestamp time.Time) (db.Context, []byte) {
	ctx := client.GetContext([]byte{DBSecondaryContext, DBSecondaryModelParentPrefix})
	ts := NewTimestamp(timestamp)
	key := append(parentID, ts[:]...)
	return ctx, key
}

// ==========================
// Primary User operations
// ==========================
func (client *DatabaseClient) SetUser(txn db.Transaction, user *User) error {
	ctx, key := client.getKeyForPrimaryUser(user.ID.Bytes())
	userBytes, err := json.Marshal(user)
	if err != nil {
		return errors.Wrap(err, "SetUser: failed to marshal user")
	}
	return txn.Set(key, userBytes, ctx)
}

func (client *DatabaseClient) DeleteUser(txn db.Transaction, userID db.Digest) error {
	ctx, key := client.getKeyForPrimaryUser(userID.Bytes())
	return txn.Delete(key, ctx)
}

func (client *DatabaseClient) GetUser(txn db.Transaction, userID db.Digest) (*User, error) {
	ctx, key := client.getKeyForPrimaryUser(userID.Bytes())
	userBytes, err := txn.Get(key, ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetUser: failed to get user")
	}
	if userBytes == nil {
		return nil, nil
	}

	var user User
	err = json.Unmarshal(userBytes, &user)
	if err != nil {
		return nil, errors.Wrap(err, "GetUser: failed to unmarshal user")
	}
	return &user, nil
}

// ==========================
// Primary Space operations
// ==========================
func (client *DatabaseClient) SetSpace(txn db.Transaction, space *Space) error {
	ctx, key := client.getKeyForPrimarySpace(space.ID.Bytes())
	spaceBytes, err := json.Marshal(space)
	if err != nil {
		return errors.Wrap(err, "SetSpace: failed to marshal space")
	}
	return txn.Set(key, spaceBytes, ctx)
}

func (client *DatabaseClient) DeleteSpace(txn db.Transaction, spaceID db.Digest) error {
	ctx, key := client.getKeyForPrimarySpace(spaceID.Bytes())
	return txn.Delete(key, ctx)
}

func (client *DatabaseClient) GetSpace(txn db.Transaction, spaceID db.Digest) (*Space, error) {
	ctx, key := client.getKeyForPrimarySpace(spaceID.Bytes())
	spaceBytes, err := txn.Get(key, ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetSpace: failed to get space")
	}
	if spaceBytes == nil {
		return nil, nil
	}

	var space Space
	err = json.Unmarshal(spaceBytes, &space)
	if err != nil {
		return nil, errors.Wrap(err, "GetSpace: failed to unmarshal space")
	}
	return &space, nil
}

// ==========================
// Primary Thread operations
// ==========================
func (client *DatabaseClient) SetThread(txn db.Transaction, thread *Thread) error {
	ctx, key := client.getKeyForPrimaryThread(thread.ID.Bytes())
	threadBytes, err := json.Marshal(thread)
	if err != nil {
		return errors.Wrap(err, "SetThread: failed to marshal thread")
	}
	return txn.Set(key, threadBytes, ctx)
}

func (client *DatabaseClient) DeleteThread(txn db.Transaction, threadID db.Digest) error {
	ctx, key := client.getKeyForPrimaryThread(threadID.Bytes())
	return txn.Delete(key, ctx)
}

func (client *DatabaseClient) GetThread(txn db.Transaction, threadID db.Digest) (*Thread, error) {
	ctx, key := client.getKeyForPrimaryThread(threadID.Bytes())
	threadBytes, err := txn.Get(key, ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetThread: failed to get thread")
	}
	var thread Thread
	err = json.Unmarshal(threadBytes, &thread)
	if err != nil {
		return nil, errors.Wrap(err, "GetThread: failed to unmarshal thread")
	}
	return &thread, nil
}

// ==========================
// Primary Message operations
// ==========================
func (client *DatabaseClient) SetMessage(txn db.Transaction, msg *CompoundMessage) error {
	ctx, key := client.getKeyForPrimaryMessage(msg.ID.Bytes())
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return errors.Wrap(err, "SetMessage: failed to marshal message")
	}
	return txn.Set(key, msgBytes, ctx)
}

func (client *DatabaseClient) DeleteMessage(txn db.Transaction, messageID db.Digest) error {
	ctx, key := client.getKeyForPrimaryMessage(messageID.Bytes())
	return txn.Delete(key, ctx)
}

func (client *DatabaseClient) GetMessage(txn db.Transaction, messageID db.Digest) (*CompoundMessage, error) {
	ctx, key := client.getKeyForPrimaryMessage(messageID.Bytes())
	msgBytes, err := txn.Get(key, ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetMessage: failed to get message")
	}
	if msgBytes == nil {
		return nil, nil
	}
	var msg CompoundMessage
	err = json.Unmarshal(msgBytes, &msg)
	if err != nil {
		return nil, errors.Wrap(err, "GetMessage: failed to unmarshal message")
	}
	return &msg, nil
}

// ==========================
// Primary Model operations
// ==========================
func (client *DatabaseClient) SetModel(txn db.Transaction, model *ModelInfo) error {
	ctx, key := client.getKeyForPrimaryModel(model.ID.Bytes())
	modelBytes, err := json.Marshal(model)
	if err != nil {
		return errors.Wrap(err, "SetModel: failed to marshal model")
	}
	return txn.Set(key, modelBytes, ctx)
}

func (client *DatabaseClient) DeleteModel(txn db.Transaction, modelID db.Digest) error {
	ctx, key := client.getKeyForPrimaryModel(modelID.Bytes())
	return txn.Delete(key, ctx)
}

func (client *DatabaseClient) GetModel(txn db.Transaction, modelID db.Digest) (*ModelInfo, error) {
	ctx, key := client.getKeyForPrimaryModel(modelID.Bytes())
	modelBytes, err := txn.Get(key, ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetModel: failed to get model")
	}
	if modelBytes == nil {
		return nil, nil
	}

	var model ModelInfo
	err = json.Unmarshal(modelBytes, &model)
	if err != nil {
		return nil, errors.Wrap(err, "GetModel: failed to unmarshal model")
	}
	return &model, nil
}

// ==========================
// Secondary User Space operations
// ==========================
func (client *DatabaseClient) SetUserSpace(txn db.Transaction, userID db.Digest, timestamp time.Time, space *Space) error {
	ctx, key := client.getKeyForSecondaryUserSpace(userID.Bytes(), timestamp)
	spaceBytes, err := json.Marshal(space)
	if err != nil {
		return errors.Wrap(err, "SetUserSpace: failed to marshal space")
	}
	return txn.Set(key, spaceBytes, ctx)
}

func (client *DatabaseClient) DeleteUserSpace(txn db.Transaction, userID db.Digest, timestamp time.Time) error {
	ctx, key := client.getKeyForSecondaryUserSpace(userID.Bytes(), timestamp)
	return txn.Delete(key, ctx)
}

func (client *DatabaseClient) GetUserSpacesReverse(txn db.Transaction, userID []byte, maxTimestamp uint64, limit int) ([]*Space, error) {
	var spaces []*Space
	ctx := client.GetContext([]byte{DBSecondaryContext, DBSecondaryUserSpacePrefix})
	it, err := txn.GetIterator(userID, ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetUserSpacesReverse: failed to get iterator")
	}
	defer it.Close()

	ts := NewTimestampFromUint64(maxTimestamp)

	if !it.Seek(ts[:]) {
		if !it.Last() {
			return spaces, nil
		}
	} else {
		if !it.Prev() {
			return spaces, nil
		}
	}

	for i := 0; i < limit && it.Valid(); i++ {
		spaceBytes, err := it.Value()
		if err != nil {
			return nil, errors.Wrap(err, "GetUserSpacesReverse: failed to get space")
		}
		var space Space
		if err := json.Unmarshal(spaceBytes, &space); err != nil {
			return nil, errors.Wrap(err, "GetUserSpacesReverse: failed to unmarshal space")
		}
		spaces = append(spaces, &space)
		if !it.Prev() {
			break
		}
	}
	return spaces, nil
}

// ==========================
// Secondary Space Thread operations
// ==========================
func (client *DatabaseClient) SetSpaceThread(txn db.Transaction, spaceID db.Digest, timestamp time.Time, thread *Thread) error {
	ctx, key := client.getKeyForSecondarySpaceThread(spaceID.Bytes(), timestamp)
	threadBytes, err := json.Marshal(thread)
	if err != nil {
		return errors.Wrap(err, "SetSpaceThread: failed to marshal thread")
	}
	return txn.Set(key, threadBytes, ctx)
}

func (client *DatabaseClient) DeleteSpaceThread(txn db.Transaction, spaceID db.Digest, timestamp time.Time) error {
	ctx, key := client.getKeyForSecondarySpaceThread(spaceID.Bytes(), timestamp)
	return txn.Delete(key, ctx)
}

func (client *DatabaseClient) GetSpaceThreadsReverse(txn db.Transaction, spaceID []byte, maxTimestamp uint64, limit int) ([]*Thread, error) {
	var threads []*Thread
	ctx := client.GetContext([]byte{DBSecondaryContext, DBSecondarySpaceThreadPrefix})
	it, err := txn.GetIterator(spaceID, ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetSpaceThreadsReverse: failed to get iterator")
	}
	defer it.Close()

	ts := NewTimestampFromUint64(maxTimestamp)

	if !it.Seek(ts[:]) {
		if !it.Last() {
			return threads, nil
		}
	} else {
		if !it.Prev() {
			return threads, nil
		}
	}

	for i := 0; i < limit && it.Valid(); i++ {
		threadBytes, err := it.Value()
		if err != nil {
			return nil, errors.Wrap(err, "GetSpaceThreadsReverse: failed to get thread")
		}
		var thread Thread
		if err := json.Unmarshal(threadBytes, &thread); err != nil {
			return nil, errors.Wrap(err, "GetSpaceThreadsReverse: failed to unmarshal thread")
		}
		threads = append(threads, &thread)
		if !it.Prev() {
			break
		}
	}
	return threads, nil
}

// ==========================
// Secondary Thread Messages operations
// ==========================
func (client *DatabaseClient) SetThreadMessage(txn db.Transaction, threadID db.Digest, timestamp time.Time, message *CompoundMessage) error {
	ctx, key := client.getKeyForSecondaryThreadMessageTimestamp(threadID.Bytes(), timestamp)
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return errors.Wrap(err, "SetThreadMessage: failed to marshal message")
	}
	return txn.Set(key, messageBytes, ctx)
}

func (client *DatabaseClient) DeleteThreadMessage(txn db.Transaction, threadID db.Digest, timestamp time.Time) error {
	ctx, key := client.getKeyForSecondaryThreadMessageTimestamp(threadID.Bytes(), timestamp)
	return txn.Delete(key, ctx)
}

func (client *DatabaseClient) GetThreadMessagesReverse(txn db.Transaction, threadID []byte, maxTimestamp uint64, limit int) ([]*CompoundMessage, error) {
	var messages []*CompoundMessage
	ctx := client.GetContext([]byte{DBSecondaryContext, DBSecondaryThreadMessageTimestampPrefix})
	it, err := txn.GetIterator(threadID, ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetThreadMessagesReverse: failed to get iterator")
	}
	defer it.Close()

	ts := NewTimestampFromUint64(maxTimestamp)

	if !it.Seek(ts[:]) {
		if !it.Last() {
			return messages, nil
		}
	} else {
		if !it.Prev() {
			return messages, nil
		}
	}

	for i := 0; i < limit && it.Valid(); i++ {
		messageBytes, err := it.Value()
		if err != nil {
			return nil, errors.Wrap(err, "GetThreadMessagesReverse: failed to get message")
		}
		var message CompoundMessage
		if err := json.Unmarshal(messageBytes, &message); err != nil {
			return nil, errors.Wrap(err, "GetThreadMessagesReverse: failed to unmarshal message")
		}
		messages = append(messages, &message)
		if !it.Prev() {
			break
		}
	}
	return messages, nil
}

// ==========================
// Secondary User Model operations
// ==========================
func (client *DatabaseClient) SetUserModel(txn db.Transaction, userID db.Digest, timestamp time.Time, model *ModelInfo) error {
	ctx, key := client.getKeyForSecondaryUserModel(userID.Bytes(), timestamp)
	modelBytes, err := json.Marshal(model)
	if err != nil {
		return errors.Wrap(err, "SetUserModel: failed to marshal model")
	}
	return txn.Set(key, modelBytes, ctx)
}

func (client *DatabaseClient) DeleteUserModel(txn db.Transaction, userID db.Digest, timestamp time.Time) error {
	ctx, key := client.getKeyForSecondaryUserModel(userID.Bytes(), timestamp)
	return txn.Delete(key, ctx)
}

func (client *DatabaseClient) GetUserModelsReverse(txn db.Transaction, userID []byte, maxTimestamp uint64, limit int) ([]*ModelInfo, error) {
	var models []*ModelInfo
	ctx := client.GetContext([]byte{DBSecondaryContext, DBSecondaryUserModelPrefix})
	it, err := txn.GetIterator(userID, ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetUserModelsReverse: failed to get iterator")
	}
	defer it.Close()

	ts := NewTimestampFromUint64(maxTimestamp)

	if !it.Seek(ts[:]) {
		if !it.Last() {
			return models, nil
		}
	} else {
		if !it.Prev() {
			return models, nil
		}
	}

	for i := 0; i < limit && it.Valid(); i++ {
		modelBytes, err := it.Value()
		if err != nil {
			return nil, errors.Wrap(err, "GetUserModelsReverse: failed to get model")
		}
		var model ModelInfo
		if err := json.Unmarshal(modelBytes, &model); err != nil {
			return nil, errors.Wrap(err, "GetUserModelsReverse: failed to unmarshal model")
		}
		models = append(models, &model)
		if !it.Prev() {
			break
		}
	}
	return models, nil
}

func (client *DatabaseClient) SetModelParent(txn db.Transaction, parentID db.Digest, timestamp time.Time, model *ModelInfo) error {
	ctx, key := client.getKeyForSecondaryModelParent(parentID.Bytes(), timestamp)
	modelBytes, err := json.Marshal(model)
	if err != nil {
		return errors.Wrap(err, "SetModelParent: failed to marshal model")
	}
	return txn.Set(key, modelBytes, ctx)
}

func (client *DatabaseClient) DeleteModelParent(txn db.Transaction, parentID db.Digest, timestamp time.Time) error {
	ctx, key := client.getKeyForSecondaryModelParent(parentID.Bytes(), timestamp)
	return txn.Delete(key, ctx)
}

func (client *DatabaseClient) GetModelsByParentReverse(txn db.Transaction, parentID []byte, maxTimestamp uint64, limit int) ([]*ModelInfo, error) {
	var models []*ModelInfo
	ctx := client.GetContext([]byte{DBSecondaryContext, DBSecondaryModelParentPrefix})
	it, err := txn.GetIterator(parentID, ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetModelsByParentReverse: failed to get iterator")
	}
	defer it.Close()

	ts := NewTimestampFromUint64(maxTimestamp)

	if !it.Seek(ts[:]) {
		if !it.Last() {
			return models, nil
		}
	} else {
		if !it.Prev() {
			return models, nil
		}
	}

	for i := 0; i < limit && it.Valid(); i++ {
		modelBytes, err := it.Value()
		if err != nil {
			return nil, errors.Wrap(err, "GetModelsByParentReverse: failed to get model")
		}
		var model ModelInfo
		if err := json.Unmarshal(modelBytes, &model); err != nil {
			return nil, errors.Wrap(err, "GetModelsByParentReverse: failed to unmarshal model")
		}
		models = append(models, &model)
		if !it.Prev() {
			break
		}
	}
	return models, nil
}

func makeTimeKey(timestamp time.Time) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, uint64(timestamp.UnixNano()))
	return key
}

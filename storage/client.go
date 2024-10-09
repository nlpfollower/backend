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
	// DBSecondaryContext is used to store secondary indexes, derived from primary data
	DBSecondaryContext byte = 0x01

	// Prefixes for primary data
	// DBUserPrefix is used to store user data
	// <DBPrimaryContext><DBUserPrefix><UserID> -> <User>
	DBPrimaryUserPrefix byte = 0x00
	// DBThreadPrefix is used to store thread data
	// <DBPrimaryContext><DBThreadPrefix><ThreadID> -> <Thread>
	DBPrimaryThreadPrefix byte = 0x01
	// DBMessagePrefix is used to store message data
	// <DBPrimaryContext><DBMessagePrefix><MessageID> -> <Message>
	DBPrimaryMessagePrefix byte = 0x02

	// Prefixes for secondary indexes
	// DBSecondaryUserThreadTimestampPrefix is used to store user's threads by timestamp
	// <DBSecondaryContext><DBSecondaryUserThreadTimestampPrefix><UserID><Timestamp> -> <Thread>
	DBSecondaryUserThreadTimestampPrefix byte = 0x00
	// DBSecondaryThreadMessageTimestampPrefix is used to store thread's messages by timestamp
	// <DBSecondaryContext><DBSecondaryThreadMessageTimestampPrefix><ThreadID><Timestamp> -> <CompositeMessage>
	DBSecondaryThreadMessageTimestampPrefix byte = 0x01
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

func (client *DatabaseClient) getKeyForPrimaryThread(threadID []byte) (db.Context, []byte) {
	ctx := client.GetContext([]byte{DBPrimaryContext, DBPrimaryThreadPrefix})
	return ctx, threadID
}

func (client *DatabaseClient) getKeyForPrimaryMessage(messageID []byte) (db.Context, []byte) {
	ctx := client.GetContext([]byte{DBPrimaryContext, DBPrimaryMessagePrefix})
	return ctx, messageID
}

func (client *DatabaseClient) getKeyForSecondaryUserThreadTimestamp(userID []byte, timestamp time.Time) (db.Context, []byte) {
	ctx := client.GetContext([]byte{DBSecondaryContext, DBSecondaryUserThreadTimestampPrefix})
	ts := NewTimestamp(timestamp)
	key := append(userID, ts[:]...)
	return ctx, key
}

func (client *DatabaseClient) getKeyForSecondaryThreadMessageTimestamp(threadID []byte, timestamp time.Time) (db.Context, []byte) {
	ctx := client.GetContext([]byte{DBSecondaryContext, DBSecondaryThreadMessageTimestampPrefix})
	ts := NewTimestamp(timestamp)
	key := append(threadID, ts[:]...)
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

func (client *DatabaseClient) DeleteUser(txn db.Transaction, userID Digest) error {
	ctx, key := client.getKeyForPrimaryUser(userID.Bytes())
	return txn.Delete(key, ctx)
}

func (client *DatabaseClient) GetUser(txn db.Transaction, userID Digest) (*User, error) {
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

func (client *DatabaseClient) DeleteThread(txn db.Transaction, threadID Digest) error {
	ctx, key := client.getKeyForPrimaryThread(threadID.Bytes())
	return txn.Delete(key, ctx)
}

func (client *DatabaseClient) GetThread(txn db.Transaction, threadID Digest) (*Thread, error) {
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

func (client *DatabaseClient) DeleteMessage(txn db.Transaction, messageID Digest) error {
	ctx, key := client.getKeyForPrimaryMessage(messageID.Bytes())
	return txn.Delete(key, ctx)
}

func (client *DatabaseClient) GetMessage(txn db.Transaction, messageID Digest) (*CompoundMessage, error) {
	ctx, key := client.getKeyForPrimaryMessage(messageID.Bytes())
	msgBytes, err := txn.Get(key, ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetMessage: failed to get message")
	}
	var msg CompoundMessage
	err = json.Unmarshal(msgBytes, &msg)
	if err != nil {
		return nil, errors.Wrap(err, "GetMessage: failed to unmarshal message")
	}
	return &msg, nil
}

// ==========================
// Secondary User Thread operations
// ==========================
func (client *DatabaseClient) SetUserThread(txn db.Transaction, userID Digest, timestamp time.Time, thread *Thread) error {
	ctx, key := client.getKeyForSecondaryUserThreadTimestamp(userID.Bytes(), timestamp)
	threadBytes, err := json.Marshal(thread)
	if err != nil {
		return errors.Wrap(err, "SetUserThread: failed to marshal thread")
	}
	return txn.Set(key, threadBytes, ctx)
}

func (client *DatabaseClient) DeleteUserThread(txn db.Transaction, userID Digest, timestamp time.Time) error {
	ctx, key := client.getKeyForSecondaryUserThreadTimestamp(userID.Bytes(), timestamp)
	return txn.Delete(key, ctx)
}

func (client *DatabaseClient) GetUserThreadsReverse(txn db.Transaction, userID []byte, maxTimestamp uint64, limit int) ([]*Thread, error) {
	var threads []*Thread
	ctx := client.GetContext([]byte{DBSecondaryContext, DBSecondaryUserThreadTimestampPrefix})
	it, err := txn.GetIterator(userID, ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetUserThreadsReverse: failed to get iterator")
	}
	defer it.Close()

	ts := NewTimestampFromUint64(maxTimestamp)

	if !it.Seek(ts[:]) {
		if !it.Last() {
			return threads, nil
		}
	} else {
		// Prev so we don't include maxTimestamp.
		if !it.Prev() {
			return threads, nil
		}
	}

	for i := 0; i < limit && it.Valid(); i++ {
		threadBytes, err := it.Value()
		if err != nil {
			return nil, errors.Wrap(err, "GetUserThreadsReverse: failed to get thread")
		}
		var thread Thread
		if err := json.Unmarshal(threadBytes, &thread); err != nil {
			return nil, errors.Wrap(err, "GetUserThreadsReverse: failed to unmarshal thread")
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

func (client *DatabaseClient) SetThreadMessage(txn db.Transaction, threadID Digest, timestamp time.Time, message *CompoundMessage) error {
	ctx, key := client.getKeyForSecondaryThreadMessageTimestamp(threadID.Bytes(), timestamp)
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return errors.Wrap(err, "SetThreadMessage: failed to marshal message")
	}
	return txn.Set(key, messageBytes, ctx)
}

func (client *DatabaseClient) DeleteThreadMessage(txn db.Transaction, threadID Digest, timestamp time.Time) error {
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
			return messages, nil // No matching keys
		}
	} else {
		if !it.Prev() {
			return messages, nil // No matching keys
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

func makeTimeKey(timestamp time.Time) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, uint64(timestamp.UnixNano()))
	return key
}

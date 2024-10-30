package storage

import (
	"encoding/binary"
	"github.com/nlpfollower/deltamind/database/db"
	"time"
)

// AuthMethod represents the method used for authentication
type AuthMethod string

const (
	AuthMethodEmailPassword AuthMethod = "email_password"
	AuthMethodEmailLink     AuthMethod = "email_link"
	AuthMethodGmail         AuthMethod = "gmail"
)

type User struct {
	ID            db.Digest  `json:"id"`
	Email         string     `json:"email"`
	Username      string     `json:"username"`
	PasswordHash  []byte     `json:"password_hash"`
	EncryptedSeed []byte     `json:"encrypted_seed"`
	AuthMethod    AuthMethod `json:"auth_method"`
	CreatedAt     time.Time  `json:"created_at"`
}

type Space struct {
	ID          db.Digest `json:"id"`
	UserID      db.Digest `json:"user_id"`
	ContentID   db.Digest `json:"content_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Thread struct {
	ID        db.Digest `json:"id"`
	SpaceID   db.Digest `json:"space_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CompoundMessageID struct {
	ID        db.Digest `json:"id"`
	MessageID uint64    `json:"message_id"`
}

type CompoundMessage struct {
	ID        db.Digest          `json:"id"`
	ThreadID  db.Digest          `json:"thread_id"`
	ParentID  *CompoundMessageID `json:"parent_id,omitempty"`
	Author    string             `json:"author"`
	Messages  []string           `json:"messages"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type ModelInfo struct {
	ID        db.Digest `json:"id"`
	UserID    db.Digest `json:"user_id"`
	ContentID db.Digest `json:"content_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ModelIteration struct {
	ID          db.Digest `json:"id"`
	ModelID     db.Digest `json:"model_id"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	Index       uint64    `json:"index"`
}

func NewKeyFromUint64(value uint64) db.Digest {
	var d db.Digest
	binary.BigEndian.PutUint64(d[:8], value)
	return d
}

type Timestamp [8]byte

func NewTimestamp(t time.Time) Timestamp {
	var ts Timestamp
	binary.BigEndian.PutUint64(ts[:], uint64(t.UnixNano()))
	return ts
}

func NewTimestampFromUint64(value uint64) Timestamp {
	var ts Timestamp
	binary.BigEndian.PutUint64(ts[:], value)
	return ts
}

func (ts Timestamp) Time() time.Time {
	return time.Unix(0, int64(binary.BigEndian.Uint64(ts[:])))
}

func (ts Timestamp) Uint64() uint64 {
	return binary.BigEndian.Uint64(ts[:])
}

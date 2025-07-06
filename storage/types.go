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
	NextCloneNum  uint64     `json:"next_clone_num"`
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

type MessageAttachment struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Lines   int    `json:"lines,omitempty"`
	Content string `json:"content"`
}

// MessageType represents the type of message
type MessageType string

const (
	MessageTypeDefault  MessageType = "default"
	MessageTypeTraining MessageType = "training"
)

// Update the CompoundMessage struct to include MessageType
type CompoundMessage struct {
	ID          db.Digest           `json:"id"`
	ThreadID    db.Digest           `json:"thread_id"`
	ParentID    *CompoundMessageID  `json:"parent_id,omitempty"`
	Author      string              `json:"author"`
	Messages    []string            `json:"messages"`
	Attachments []MessageAttachment `json:"attachments,omitempty"`
	MessageType MessageType         `json:"message_type,omitempty"` // New field
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

// Helper to normalize message type
func (m *CompoundMessage) GetMessageType() MessageType {
	if m.MessageType == "" {
		return MessageTypeDefault
	}
	return m.MessageType
}

func NewKeyFromUint64(value uint64) db.Digest {
	var d db.Digest
	binary.BigEndian.PutUint64(d[:8], value)
	return d
}

type Timestamp [8]byte

type ModelType string

const (
	ModelTypeBase    ModelType = "base"
	ModelTypeClone   ModelType = "clone"
	ModelTypeTrained ModelType = "trained"
)

type ModelStatus string

const (
	ModelStatusReady    ModelStatus = "ready"
	ModelStatusTraining ModelStatus = "training"
	ModelStatusError    ModelStatus = "error"
)

type ModelInfo struct {
	ID             db.Digest   `json:"id"`
	UserID         db.Digest   `json:"user_id"`
	Name           string      `json:"name"`         // Auto-generated: "llama-70b-u1-c1-t2"
	DisplayName    string      `json:"display_name"` // User-friendly name
	ModelType      ModelType   `json:"model_type"`
	BaseModel      string      `json:"base_model"` // "llama-8b" or "llama-70b"
	ModelSize      string      `json:"model_size"` // "8B" or "70B"
	ParentID       *db.Digest  `json:"parent_id,omitempty"`
	Status         ModelStatus `json:"status"`
	CheckpointPath string      `json:"checkpoint_path"`           // Actual checkpoint location
	TrainingJobID  string      `json:"training_job_id,omitempty"` // Associated training job ID
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

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

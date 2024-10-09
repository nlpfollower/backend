package storage

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// Digest represents a 256-bit (32-byte) digest of a hash function
type Digest [32]byte

func NewDigest(data []byte) Digest {
	var d Digest
	copy(d[:], data)
	return d
}

func (d Digest) Bytes() []byte {
	return d[:]
}

func (d Digest) String() string {
	return hex.EncodeToString(d[:])
}

func DigestFromString(s string) (Digest, error) {
	bytes, err := hex.DecodeString(s)
	if err != nil {
		return Digest{}, err
	}
	if len(bytes) != 32 {
		return Digest{}, fmt.Errorf("invalid digest length: got %d, want 32", len(bytes))
	}
	return NewDigest(bytes), nil
}

// AuthMethod represents the method used for authentication
type AuthMethod string

const (
	AuthMethodEmailPassword AuthMethod = "email_password"
	AuthMethodEmailLink     AuthMethod = "email_link"
	AuthMethodGmail         AuthMethod = "gmail"
)

type User struct {
	ID            Digest     `json:"id"`
	Email         string     `json:"email"`
	Username      string     `json:"username"`
	PasswordHash  []byte     `json:"password_hash"`
	EncryptedSeed []byte     `json:"encrypted_seed"`
	AuthMethod    AuthMethod `json:"auth_method"`
	CreatedAt     time.Time  `json:"created_at"`
}

type Thread struct {
	ID        Digest    `json:"id"`
	UserID    Digest    `json:"user_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CompoundMessageID struct {
	ID        Digest `json:"id"`
	MessageID int    `json:"message_id"`
}

type CompoundMessage struct {
	ID        Digest             `json:"id"`
	ThreadID  Digest             `json:"thread_id"`
	ParentID  *CompoundMessageID `json:"parent_id,omitempty"`
	Author    string             `json:"author"`
	Messages  []string           `json:"content"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

func NewKeyFromUint64(value uint64) Digest {
	var d Digest
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

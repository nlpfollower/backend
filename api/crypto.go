package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/sha3"
	"io"
)

// GetUserIDFromEmail returns a 32-byte hash of the given email address.
// The hash is computed as SHA3(SHA3(email) || SHA3(salt)).
func GetUserIDFromEmail(email string) []byte {
	h256 := sha3.New256()
	h256.Write(SaltBytes(email))
	emailHash := h256.Sum(nil)

	h256.Reset()
	h256.Write([]byte(DMHashSaltEmail))
	saltHash := h256.Sum(nil)

	h256.Reset()
	h256.Write(emailHash)
	h256.Write(saltHash)
	return h256.Sum(nil)
}

func HashPassword(password string) []byte {
	return pbkdf2.Key([]byte(password), SaltBytes(DMSaltPassword), PasswordHashIter, PasswordHashKeyLen, sha256.New)
}

func GenerateRandomSalt() ([]byte, error) {
	salt := make([]byte, SaltLength)
	_, err := rand.Read(salt)
	return salt, err
}

func GenerateRandomSeed() ([]byte, error) {
	seed := make([]byte, SeedLength)
	_, err := rand.Read(seed)
	return seed, err
}

func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func EncryptSeed(seed, password []byte) ([]byte, error) {
	key := pbkdf2.Key(password, SaltBytes(DMSaltSeed), 4096, AESKeySize, sha3.New256)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, seed, nil), nil
}

func DecryptSeed(encryptedSeed, password []byte) ([]byte, error) {
	key := pbkdf2.Key(password, SaltBytes(DMSaltSeed), 4096, AESKeySize, sha3.New256)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(encryptedSeed) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := encryptedSeed[:nonceSize], encryptedSeed[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

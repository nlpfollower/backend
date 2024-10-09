package api

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"time"
)

type AuthToken struct {
	SessionKey string    `json:"session_key"`
	UserID     string    `json:"user_id"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type SessionClaims struct {
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
	jwt.RegisteredClaims
}

func GenerateAuthToken(userID string) (*AuthToken, error) {
	sessionKey, err := GenerateSessionKey(userID)
	if err != nil {
		return nil, err
	}

	return &AuthToken{
		SessionKey: sessionKey,
		UserID:     userID,
		ExpiresAt:  time.Now().Add(SessionDuration),
	}, nil
}

func GenerateSessionKey(userID string) (string, error) {
	// Generate a random session ID
	sessionID := make([]byte, 32)
	if _, err := rand.Read(sessionID); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Create the claims
	now := time.Now()
	claims := SessionClaims{
		UserID:    userID,
		SessionID: base64.RawURLEncoding.EncodeToString(sessionID),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(SessionDuration)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign the token
	signedToken, err := token.SignedString([]byte(SecretKey))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	// Encode the signed token
	encodedToken := base64.RawURLEncoding.EncodeToString([]byte(signedToken))

	// Combine prefix and encoded token
	sessionKey := fmt.Sprintf("%s%s", SessionKeyPrefix, encodedToken)

	return sessionKey, nil
}

func ValidateSessionKey(sessionKey string) (*SessionClaims, error) {
	// Remove prefix
	if len(sessionKey) <= len(SessionKeyPrefix) || sessionKey[:len(SessionKeyPrefix)] != SessionKeyPrefix {
		return nil, fmt.Errorf("invalid session key format")
	}
	encodedToken := sessionKey[len(SessionKeyPrefix):]

	// Decode the token
	decodedToken, err := base64.RawURLEncoding.DecodeString(encodedToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decode session key: %w", err)
	}

	// Parse and validate the token
	claims := &SessionClaims{}
	token, err := jwt.ParseWithClaims(string(decodedToken), claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(SecretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse session key: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid session key")
	}

	return claims, nil
}

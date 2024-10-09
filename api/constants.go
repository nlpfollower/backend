package api

import "time"

const (
	SessionKeyPrefix = "sk-dm-"            // dm for DeltaMind
	SessionDuration  = 45 * 24 * time.Hour // 45 days
	SecretKey        = "your-secret-key"   // Replace with a secure, environment-specific secret

	SeedLength         = 32
	PasswordHashIter   = 100000
	PasswordHashKeyLen = 32
	SaltLength         = 16
	AESKeySize         = 32

	DMHashSaltEmail = "dm-salt-email"
	DMSaltPassword  = "dm-salt-password"
	DMSaltSeed      = "dm-salt-seed"
)

func SaltBytes(salt string) []byte {
	return []byte(salt)
}

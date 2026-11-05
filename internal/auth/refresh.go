package auth

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
)

const RefreshTokenTTL = 7 * 24 * time.Hour

// GenerateRefreshToken creates a cryptographically random opaque token.
func GenerateRefreshToken() (string, error) {
	bytes := make([]byte, 32)

	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// NewRefreshTokenFamily creates a unique token family ID.
func NewRefreshTokenFamily() string {
	return uuid.NewString()
}

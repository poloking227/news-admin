package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// RefreshTTL is the lifetime of a refresh token.
const RefreshTTL = 7 * 24 * time.Hour

// NewRefreshJTI returns a cryptographically random refresh token id. The jti
// itself acts as the bearer token: it is stored hashed-in-a-cookie only via
// the session row and never logged.
func NewRefreshJTI() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate refresh jti: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

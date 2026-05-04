package middleware

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// hashAPIKeyInternal hashes a plaintext API key with SHA-256.
// Duplicated from usecase package intentionally to avoid circular imports.
func hashAPIKeyInternal(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// generateRequestID creates a 16-byte cryptographically random request ID.
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based ID if crypto/rand fails
		return hex.EncodeToString([]byte(time.Now().String()))[:32]
	}
	return hex.EncodeToString(b)
}

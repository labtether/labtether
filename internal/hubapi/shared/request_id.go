package shared

import (
	"crypto/rand"
	"encoding/base64"
)

// GenerateRequestID creates a unique request ID for agent commands,
// file operations, bridge sessions, and other correlation purposes.
func GenerateRequestID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

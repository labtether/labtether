package terminal

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// TmuxSessionNameForID derives a stable, shell-safe tmux name with 128 bits of
// collision resistance. LabTether IDs begin with a timestamp and end with the
// uniqueness counter, so truncating the raw prefix can discard exactly the
// bytes that distinguish concurrently-created sessions.
func TmuxSessionNameForID(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return "lt-shell"
	}
	sum := sha256.Sum256([]byte(trimmed))
	return "lt-" + hex.EncodeToString(sum[:16])
}

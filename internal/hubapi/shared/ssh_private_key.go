package shared

import "strings"

// NormalizePrivateKey trims whitespace and converts literal "\n"
// escape sequences to real newlines, which is common when SSH keys
// are pasted into environment variables or JSON config.
func NormalizePrivateKey(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	return strings.ReplaceAll(trimmed, `\n`, "\n")
}

package sysconfig

import "strings"

const MaxCommandOutputBytes = 64 * 1024

// TruncateCommandOutput returns a trimmed string of at most maxBytes from
// payload. If the payload exceeds maxBytes it is truncated with a marker.
func TruncateCommandOutput(payload []byte, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 8 * 1024
	}
	if len(payload) <= maxBytes {
		return strings.TrimSpace(string(payload))
	}
	return strings.TrimSpace(string(payload[:maxBytes])) + "\n...output truncated"
}

// CloneStringSlice returns a shallow copy of the input slice.
func CloneStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}

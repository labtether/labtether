package shared

import (
	"regexp"
	"strings"
)

var (
	// upstreamURLPattern matches http/https/ws/wss URLs that may expose internal
	// hostnames or IP addresses in upstream error messages returned to clients.
	upstreamURLPattern = regexp.MustCompile(`(?i)(wss?|https?)://[^\s"']+`)
	// upstreamIPPortPattern matches bare IP:port or hostname:port strings that
	// may appear in dial/connect error messages.
	upstreamIPPortPattern = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}:\d+\b`)
)

const RedactedUpstreamAddr = "[upstream]"

// SanitizeUpstreamError strips internal URLs and IP:port addresses from
// upstream error messages before they are returned to API clients.
func SanitizeUpstreamError(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "upstream service unavailable"
	}
	msg = upstreamURLPattern.ReplaceAllString(msg, RedactedUpstreamAddr)
	msg = upstreamIPPortPattern.ReplaceAllString(msg, RedactedUpstreamAddr)
	return msg
}

package collectors

import (
	"regexp"
	"strings"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

const RedactedConnectorSecret = "[redacted]"

var (
	connectorSecretKVPattern      = regexp.MustCompile(`(?i)(["']?(?:token_secret|password|api_key|secret|authorization|x-api-key|token)["']?\s*[:=]\s*)("[^"]*"|'[^']*'|[^,\s}\]]+)`)
	connectorAuthorizationPattern = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)([^\s,;]+(?:\s+[^\s,;]+)?)`)
	connectorPVEAPITokenPattern   = regexp.MustCompile(`(?i)(pveapitoken=[^=\s]+)=([^\s,;]+)`)
	connectorURLCredentialPattern = regexp.MustCompile(`(?i)(https?://[^/\s:@]+:)([^@/\s]+)(@)`)
)

func SanitizeConnectorError(err error, secrets ...string) string {
	if err == nil {
		return "connection test failed"
	}
	return SanitizeConnectorErrorMessage(err.Error(), secrets...)
}

func SanitizeConnectorErrorMessage(message string, secrets ...string) string {
	sanitized := strings.TrimSpace(message)
	if sanitized == "" {
		return "connection test failed"
	}

	sanitized = connectorSecretKVPattern.ReplaceAllString(sanitized, `${1}`+RedactedConnectorSecret)
	sanitized = connectorAuthorizationPattern.ReplaceAllString(sanitized, `${1}`+RedactedConnectorSecret)
	sanitized = connectorPVEAPITokenPattern.ReplaceAllString(sanitized, `${1}=`+RedactedConnectorSecret)
	sanitized = connectorURLCredentialPattern.ReplaceAllString(sanitized, `${1}`+RedactedConnectorSecret+`${3}`)

	for _, secret := range secrets {
		trimmed := strings.TrimSpace(secret)
		if trimmed == "" {
			continue
		}
		sanitized = strings.ReplaceAll(sanitized, trimmed, RedactedConnectorSecret)
	}

	return sanitized
}

// SanitizeUpstreamError delegates to shared.SanitizeUpstreamError.
func SanitizeUpstreamError(msg string) string {
	return shared.SanitizeUpstreamError(msg)
}

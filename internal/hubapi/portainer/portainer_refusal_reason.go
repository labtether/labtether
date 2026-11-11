package portainer

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

const portainerRefusalReasonMaxRunes = 200

// Socket and daemon addresses that Docker or compose output can carry
// (shared.SanitizeUpstreamError covers http(s) and ws(s) URLs and IP:port).
var portainerSocketAddressPattern = regexp.MustCompile(`(?i)\b(unix|tcp|npipe|ssh)://[^\s"']+`)

// portainerRefusalReason makes Portainer's own reason safe to show any user
// who may call the route: addresses and URLs (with any user:token@ in them)
// are replaced, control characters and line breaks become spaces, and the
// text is one line of at most 200 characters, cut with "…".
func portainerRefusalReason(reason string) string {
	reason = shared.SanitizeUpstreamError(reason)
	reason = portainerSocketAddressPattern.ReplaceAllString(reason, shared.RedactedUpstreamAddr)
	reason = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, reason)
	reason = strings.Join(strings.Fields(reason), " ")
	if utf8.RuneCountInString(reason) > portainerRefusalReasonMaxRunes {
		return strings.TrimSpace(string([]rune(reason)[:portainerRefusalReasonMaxRunes-1])) + "…"
	}
	return strings.TrimSuffix(reason, ".") + "."
}

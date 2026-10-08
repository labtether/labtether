package securityruntime

import (
	"regexp"
	"strings"
)

var urlUserinfoInTextPattern = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/?#\r\n]*@`)

// URLContainsUserinfo reports whether rawURL has an authority containing an
// embedded username or password. It deliberately recognizes malformed URL
// escapes too, so invalid input cannot bypass secret filtering.
func URLContainsUserinfo(rawURL string) bool {
	_, _, ok := urlUserinfoBounds(rawURL)
	return ok
}

// RedactURLUserinfo removes an embedded username and password from a URL
// authority. The boolean result is false when the value has no userinfo, in
// which case the original value is returned unchanged.
func RedactURLUserinfo(rawURL string) (string, bool) {
	trimmed := strings.TrimSpace(rawURL)
	authorityStart, userinfoEnd, ok := urlUserinfoBounds(trimmed)
	if !ok {
		return rawURL, false
	}
	return trimmed[:authorityStart] + trimmed[userinfoEnd:], true
}

// RedactURLUserinfoInText removes URL usernames and passwords from diagnostic
// text without exposing or interpreting the credential bytes.
func RedactURLUserinfoInText(value string) (string, bool) {
	redacted := urlUserinfoInTextPattern.ReplaceAllString(value, "${1}")
	return redacted, redacted != value
}

// RedactURLUserinfoValues returns a cloned string map with URL userinfo removed
// from every value. Callers can safely redact a response without mutating the
// stored metadata that supplied it.
func RedactURLUserinfoValues(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	redacted := make(map[string]string, len(values))
	for key, value := range values {
		if safeValue, changed := RedactURLUserinfo(value); changed {
			redacted[key] = safeValue
		} else {
			redacted[key] = value
		}
	}
	return redacted
}

func urlUserinfoBounds(rawURL string) (authorityStart int, userinfoEnd int, ok bool) {
	trimmed := strings.TrimSpace(rawURL)
	if strings.HasPrefix(trimmed, "//") {
		authorityStart = 2
	} else {
		schemeEnd := strings.Index(trimmed, "://")
		if schemeEnd <= 0 {
			return 0, 0, false
		}
		for index, char := range trimmed[:schemeEnd] {
			if index == 0 {
				if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') {
					return 0, 0, false
				}
				continue
			}
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '+' && char != '-' && char != '.' {
				return 0, 0, false
			}
		}
		authorityStart = schemeEnd + 3
	}

	authorityEnd := len(trimmed)
	if suffix := strings.IndexAny(trimmed[authorityStart:], "/?#"); suffix >= 0 {
		authorityEnd = authorityStart + suffix
	}
	userinfoAt := strings.LastIndex(trimmed[authorityStart:authorityEnd], "@")
	if userinfoAt < 0 {
		return 0, 0, false
	}
	return authorityStart, authorityStart + userinfoAt + 1, true
}

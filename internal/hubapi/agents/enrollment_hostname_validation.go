package agents

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxEnrollmentHostnameBytes = 253

func validEnrollmentHostname(raw string, allowEmpty bool) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return value, allowEmpty
	}
	if len(value) > maxEnrollmentHostnameBytes || !utf8.ValidString(value) {
		return "", false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return value, true
}

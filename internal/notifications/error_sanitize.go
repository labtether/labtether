package notifications

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxSanitizedDeliveryErrorLength = 512

var (
	deliveryErrorURLPattern      = regexp.MustCompile(`(?i)\b(?:https?|wss?)://[^\s"'<>]+`)
	deliveryErrorAuthPattern     = regexp.MustCompile(`(?i)\b(authorization|proxy-authorization)\s*[:=]\s*(?:bearer|basic)\s+[^\s,;]+`)
	deliveryErrorSMTPAuthPattern = regexp.MustCompile(`(?i)\bAUTH\s+(?:PLAIN|LOGIN|CRAM-MD5)\s+[^\s,;]+`)
	deliveryErrorKVPattern       = regexp.MustCompile(`(?i)\b(authorization|proxy-authorization|password|passwd|secret|token|api[_-]?key|client[_-]?secret)\s*[:=]\s*[^\s,;]+`)
	deliveryCipherPattern        = regexp.MustCompile(`\bv2:[A-Za-z0-9_+/=-]+`)
)

// SanitizeDeliveryError returns a bounded operator-facing error that cannot
// disclose URL paths/query strings, common credential key/value pairs, or
// encrypted secret material. Callers with channel config should first replace
// the exact configured secret values, then pass the result through the message
// variant below.
func SanitizeDeliveryError(err error) string {
	if err == nil {
		return ""
	}
	return SanitizeDeliveryErrorMessage(err.Error())
}

func SanitizeDeliveryErrorMessage(message string) string {
	if strings.TrimSpace(message) == "" {
		return ""
	}
	message = deliveryErrorURLPattern.ReplaceAllStringFunc(message, sanitizeDeliveryErrorURL)
	message = deliveryErrorAuthPattern.ReplaceAllString(message, "$1=[redacted]")
	message = deliveryErrorSMTPAuthPattern.ReplaceAllString(message, "AUTH [redacted]")
	message = deliveryErrorKVPattern.ReplaceAllString(message, "$1=[redacted]")
	message = deliveryCipherPattern.ReplaceAllString(message, "[redacted-ciphertext]")
	message = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, message)
	message = strings.Join(strings.Fields(message), " ")
	if message == "" {
		message = "notification delivery failed"
	}
	if len(message) > maxSanitizedDeliveryErrorLength {
		cut := maxSanitizedDeliveryErrorLength
		for cut > 0 && !utf8.RuneStart(message[cut]) {
			cut--
		}
		message = message[:cut]
	}
	return message
}

func sanitizeDeliveryErrorURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.Hostname() == "" {
		return "[redacted-url]"
	}
	host := parsed.Hostname()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port := parsed.Port(); port != "" {
		host += ":" + port
	}
	return strings.ToLower(parsed.Scheme) + "://" + host + "/[redacted]"
}

package securityruntime

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"unicode"
)

const (
	envLogSanitizeMaxLength = "LABTETHER_LOG_SANITIZE_MAX_LEN"
	defaultLogMaxLength     = 512
	minLogMaxLength         = 64
	maxLogMaxLength         = 4096
)

func logSanitizeMaxLength() int {
	raw := strings.TrimSpace(os.Getenv(envLogSanitizeMaxLength))
	if raw == "" {
		return defaultLogMaxLength
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return defaultLogMaxLength
	}
	if parsed < minLogMaxLength {
		return minLogMaxLength
	}
	if parsed > maxLogMaxLength {
		return maxLogMaxLength
	}
	return parsed
}

func SanitizeLogValue(raw string) string {
	if raw == "" {
		return ""
	}

	maxLen := logSanitizeMaxLength()
	var builder strings.Builder
	builder.Grow(len(raw))

	for _, r := range raw {
		switch r {
		case '\n':
			builder.WriteString("\\n")
		case '\r':
			builder.WriteString("\\r")
		case '\t':
			builder.WriteString("\\t")
		default:
			if unicode.IsControl(r) {
				builder.WriteRune('?')
			} else {
				builder.WriteRune(r)
			}
		}
		if builder.Len() >= maxLen {
			break
		}
	}

	out := builder.String()
	if len(out) > maxLen {
		out = out[:maxLen]
	}
	if len(out) < len(raw) {
		out += "...(truncated)"
	}
	return out
}

func SanitizeLogArg(value any) any {
	switch typed := value.(type) {
	case string:
		return SanitizeLogValue(typed)
	case error:
		return SanitizeLogValue(typed.Error())
	case []byte:
		return SanitizeLogValue(string(typed))
	case fmt.Stringer:
		return SanitizeLogValue(typed.String())
	default:
		return value
	}
}

func SanitizeLogArgs(args ...any) []any {
	if len(args) == 0 {
		return nil
	}
	sanitized := make([]any, 0, len(args))
	for _, arg := range args {
		sanitized = append(sanitized, SanitizeLogArg(arg))
	}
	return sanitized
}

func Logf(format string, args ...any) {
	// #nosec G706 -- string-like inputs are sanitized before logging to neutralize control characters/newlines.
	log.Printf(format, SanitizeLogArgs(args...)...)
}

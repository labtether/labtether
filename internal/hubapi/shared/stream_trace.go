package shared

import (
	"net/http"
	"strings"
)

const (
	StreamTraceQueryKey = "lt_trace"
	StreamTraceMaxLen   = 64
)

func BrowserStreamTraceID(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return SanitizeBrowserStreamTraceID(r.URL.Query().Get(StreamTraceQueryKey))
}

func StreamTraceLogValue(traceID string) string {
	if traceID == "" {
		return "-"
	}
	return traceID
}

func SanitizeBrowserStreamTraceID(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if len(value) > StreamTraceMaxLen {
		value = value[:StreamTraceMaxLen]
	}

	var b strings.Builder
	b.Grow(len(value))
	for _, ch := range value {
		switch {
		case ch >= 'a' && ch <= 'z':
			b.WriteRune(ch)
		case ch >= 'A' && ch <= 'Z':
			b.WriteRune(ch)
		case ch >= '0' && ch <= '9':
			b.WriteRune(ch)
		case ch == '-' || ch == '_' || ch == '.':
			b.WriteRune(ch)
		}
	}
	return b.String()
}

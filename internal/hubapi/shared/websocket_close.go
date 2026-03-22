package shared

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

// MaxWebSocketCloseReasonBytes is the maximum number of bytes allowed in
// a WebSocket close frame reason field (RFC 6455 section 5.5.1).
const MaxWebSocketCloseReasonBytes = 123

// NormalizeWebSocketCloseReason trims and truncates a close-frame reason
// string so it fits within the WebSocket 125-byte control frame limit.
func NormalizeWebSocketCloseReason(reason string) string {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return ""
	}
	if len([]byte(trimmed)) <= MaxWebSocketCloseReasonBytes {
		return trimmed
	}
	var b strings.Builder
	b.Grow(MaxWebSocketCloseReasonBytes)
	for _, r := range trimmed {
		size := utf8.RuneLen(r)
		if size <= 0 {
			size = 1
		}
		if b.Len()+size > MaxWebSocketCloseReasonBytes {
			break
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// SafeWriteClose sends a WebSocket close control frame with the given
// status code and reason. If the reason is too long, it is truncated.
// If the first write fails, a second attempt is made with an empty
// reason to maximise the chance the peer receives a clean close.
func SafeWriteClose(wsConn *websocket.Conn, code int, reason string) {
	if wsConn == nil {
		return
	}
	normalizedReason := NormalizeWebSocketCloseReason(reason)
	deadline := time.Now().Add(time.Second)
	if err := wsConn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(code, normalizedReason),
		deadline,
	); err == nil {
		return
	}
	_ = wsConn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(code, ""),
		deadline,
	)
}

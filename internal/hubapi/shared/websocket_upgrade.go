package shared

import (
	"errors"
	"net/http"

	"github.com/gorilla/websocket"
)

var (
	// ErrWebSocketUpgraderRequired indicates that no WebSocket upgrader was configured.
	ErrWebSocketUpgraderRequired = errors.New("websocket upgrader is required")
	// ErrWebSocketOriginPolicyRequired indicates that an upgrader lacks an explicit origin policy.
	ErrWebSocketOriginPolicyRequired = errors.New("websocket upgrader requires an explicit origin policy")
)

// UpgradeWebSocket performs an upgrade only when the caller supplied both an
// upgrader and an explicit origin-validation policy. Keeping this invariant in
// one place prevents a zero-value websocket.Upgrader from silently reaching a
// production stream handler.
func UpgradeWebSocket(
	upgrader *websocket.Upgrader,
	w http.ResponseWriter,
	r *http.Request,
	responseHeader http.Header,
) (*websocket.Conn, error) {
	if upgrader == nil {
		return nil, ErrWebSocketUpgraderRequired
	}
	if upgrader.CheckOrigin == nil {
		return nil, ErrWebSocketOriginPolicyRequired
	}

	// Origin validation is guaranteed by the explicit invariant check above.
	// nosemgrep: go.gorilla.security.audit.websocket-missing-origin-check.websocket-missing-origin-check
	return upgrader.Upgrade(w, r, responseHeader)
}

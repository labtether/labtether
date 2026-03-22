package agents

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
)

func DecodePendingEnrollmentAssetID(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req struct {
		AssetID string `json:"asset_id"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return "", false
	}
	req.AssetID = strings.TrimSpace(req.AssetID)
	if req.AssetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset_id is required")
		return "", false
	}
	return req.AssetID, true
}

func ResolveApprovedAssetID(agent *PendingAgent, pendingAssetID string) string {
	stableAssetID := NormalizeHostnameForAssetID(agent.Hostname)
	if stableAssetID == "" || stableAssetID == "unknown" {
		stableAssetID = pendingAssetID
	}
	return stableAssetID
}

// NormalizeHostnameForAssetID normalizes a hostname into a safe, stable asset ID
// using the same character rules as BuildPendingEnrollmentAssetID: lowercase,
// alphanumeric plus separator chars, truncated to a safe length.
func NormalizeHostnameForAssetID(hostname string) string {
	h := strings.ToLower(strings.TrimSpace(hostname))
	if h == "" {
		return ""
	}
	if len(h) > maxPendingHostnameIDLen {
		h = h[:maxPendingHostnameIDLen]
	}
	var b strings.Builder
	b.Grow(len(h))
	for _, ch := range h {
		switch {
		case ch >= 'a' && ch <= 'z':
			b.WriteRune(ch)
		case ch >= '0' && ch <= '9':
			b.WriteRune(ch)
		case ch == '-' || ch == '_' || ch == '.':
			b.WriteRune(ch)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func SendPendingEnrollmentDecision(agent *PendingAgent, msgType string, data any, closeReason string) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	msg := agentmgr.Message{
		Type: msgType,
		Data: payload,
	}

	agent.ConnMu.Lock()
	defer agent.ConnMu.Unlock()

	_ = agent.Conn.SetWriteDeadline(time.Now().Add(agentmgr.AgentWriteDeadline))
	writeErr := agent.Conn.WriteJSON(msg)

	if closeReason != "" {
		closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, closeReason)
		_ = agent.Conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(agentmgr.AgentWriteDeadline))
	}

	return writeErr
}

package agents

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
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

func ResolveApprovedAssetID(agent PendingAgentInfo, pendingAssetID string) string {
	stableAssetID := NormalizeHostnameForAssetID(agent.Hostname)
	if stableAssetID == "" || stableAssetID == "unknown" {
		stableAssetID = pendingAssetID
	}
	return stableAssetID
}

func SendPendingEnrollmentClaimDecision(claim PendingDecisionClaim, msgType string, data any, closeReason string) error {
	if claim.conn == nil || claim.connMu == nil {
		return fmt.Errorf("pending agent connection not available")
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	msg := agentmgr.Message{Type: msgType, Data: payload}
	claim.connMu.Lock()
	defer claim.connMu.Unlock()
	_ = claim.conn.SetWriteDeadline(time.Now().Add(agentmgr.AgentWriteDeadline))
	writeErr := claim.conn.WriteJSON(msg)
	if closeReason != "" {
		closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, closeReason)
		_ = claim.conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(agentmgr.AgentWriteDeadline))
		_ = claim.conn.Close()
	}
	return writeErr
}

// ClosePendingEnrollmentClaim terminates a successfully decided pending
// connection after its final message has been delivered. Server-side close is
// required because a noncompliant client may ignore the close control frame.
func ClosePendingEnrollmentClaim(claim PendingDecisionClaim, reason string) error {
	if claim.conn == nil || claim.connMu == nil {
		return fmt.Errorf("pending agent connection not available")
	}
	claim.connMu.Lock()
	defer claim.connMu.Unlock()
	closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, reason)
	writeErr := claim.conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(agentmgr.AgentWriteDeadline))
	closeErr := claim.conn.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func closePendingEnrollmentAgent(agent *PendingAgent, code int, reason string) {
	if agent == nil || agent.Conn == nil {
		return
	}
	agent.ConnMu.Lock()
	defer agent.ConnMu.Unlock()
	closeMsg := websocket.FormatCloseMessage(code, reason)
	_ = agent.Conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(agentmgr.AgentWriteDeadline))
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

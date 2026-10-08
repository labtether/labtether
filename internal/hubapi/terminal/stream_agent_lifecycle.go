package terminal

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
	"log"
	"strings"
	"sync"
)

func SendTerminalClose(agentConn *agentmgr.AgentConn, sessionID string) {
	_ = SendTerminalCloseWithError(agentConn, sessionID)
}

func SendTerminalCloseWithError(agentConn *agentmgr.AgentConn, sessionID string) error {
	if agentConn == nil || sessionID == "" {
		return errors.New("agent connection and terminal session id are required")
	}
	closeData, err := json.Marshal(agentmgr.TerminalCloseData{SessionID: sessionID})
	if err != nil {
		return fmt.Errorf("marshal terminal close request: %w", err)
	}
	return agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgTerminalClose,
		ID:   sessionID,
		Data: closeData,
	})
}

func (d *Deps) FinalizeAgentTerminalSession(
	sessionID string,
	bridgeState *TerminalBridge,
	agentConn *agentmgr.AgentConn,
	startSent bool,
	closeFn func(*agentmgr.AgentConn, string),
) {
	d.TerminalBridges.Delete(sessionID)
	if bridgeState != nil {
		bridgeState.Close()
	}
	if startSent {
		if closeFn == nil {
			closeFn = SendTerminalClose
		}
		closeFn(agentConn, sessionID)
	}
}

func (d *Deps) CloseTerminalBridgesForAsset(assetID string) {
	trimmedAssetID := strings.TrimSpace(assetID)
	if trimmedAssetID == "" {
		return
	}
	d.TerminalBridges.Range(func(_ any, value any) bool {
		bridge, ok := value.(*TerminalBridge)
		if !ok {
			// Probe channels share this map; only terminal bridge sessions need closure.
			return true
		}
		if bridge.MatchesAgent(trimmedAssetID) {
			bridge.CloseWithReason("agent_disconnected")
		}
		return true
	})
}

const maxTerminalInputReadBytes = 256 * 1024

// TerminalBridge holds the channels for a terminal session bridged through an agent.
type TerminalBridge struct {
	OutputCh        chan []byte
	ClosedCh        chan struct{}
	ExpectedAgentID string
	CloseMu         sync.Once
	CloseReasonMu   sync.RWMutex
	CloseReasonVal  string
	SessionID       string
	Target          string
	TraceID         string
	RequireTmux     bool
	Scrollback      *terminal.RingBuffer
}

func (b *TerminalBridge) Close() {
	b.CloseWithReason("")
}

func (b *TerminalBridge) CloseWithReason(reason string) {
	if b == nil {
		return
	}
	trimmedReason := strings.TrimSpace(reason)
	if trimmedReason != "" {
		b.CloseReasonMu.Lock()
		if b.CloseReasonVal == "" {
			b.CloseReasonVal = trimmedReason
		}
		b.CloseReasonMu.Unlock()
	}
	b.CloseMu.Do(func() {
		close(b.ClosedCh)
	})
}

func (b *TerminalBridge) CloseReasonOr(fallback string) string {
	if b == nil {
		return strings.TrimSpace(fallback)
	}
	b.CloseReasonMu.RLock()
	reason := strings.TrimSpace(b.CloseReasonVal)
	b.CloseReasonMu.RUnlock()
	if reason != "" {
		return reason
	}
	return strings.TrimSpace(fallback)
}

func (b *TerminalBridge) TrySendOutput(payload []byte) {
	if b == nil {
		return
	}
	select {
	case <-b.ClosedCh:
		return
	default:
	}
	defer func() { _ = recover() }()
	select {
	case b.OutputCh <- payload:
	default:
	}
}

func (b *TerminalBridge) MatchesAgent(assetID string) bool {
	if b == nil {
		return false
	}
	expected := strings.TrimSpace(b.ExpectedAgentID)
	if expected == "" {
		return false
	}
	return expected == strings.TrimSpace(assetID)
}

// ProcessAgentTerminalStarted handles terminal.started from agent.
func (d *Deps) ProcessAgentTerminalStarted(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.TerminalStartedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	if bridge, ok := d.TerminalBridges.Load(data.SessionID); ok {
		if b, ok := bridge.(*TerminalBridge); ok {
			if conn == nil || !b.MatchesAgent(conn.AssetID) {
				return
			}
			log.Printf("terminal-agent: agent_reported_started session=%s target=%s trace=%s tmux_attached=%t", b.SessionID, b.Target, shared.StreamTraceLogValue(b.TraceID), data.TmuxAttached)
			if b.RequireTmux && !data.TmuxAttached {
				conn.SetMeta("terminal.tmux.has", "false")
				conn.SetMeta("terminal.tmux.path", "")
				b.CloseWithReason("tmux_unavailable")
				return
			}
			b.TrySendOutput(nil) // nil = started marker
		}
	}
}

// ProcessAgentTerminalData handles terminal.data (output) from agent.
func (d *Deps) ProcessAgentTerminalData(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var payload agentmgr.TerminalDataPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil || len(decoded) == 0 {
		return
	}

	if bridge, ok := d.TerminalBridges.Load(payload.SessionID); ok {
		if b, ok := bridge.(*TerminalBridge); ok {
			if conn == nil || !b.MatchesAgent(conn.AssetID) {
				return
			}
			if b.Scrollback != nil {
				b.Scrollback.Write(decoded)
			}
			b.TrySendOutput(decoded)
		}
	}
}

// ProcessAgentTerminalClosed handles terminal.closed from agent.
func (d *Deps) ProcessAgentTerminalClosed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.TerminalCloseData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	if bridge, ok := d.TerminalBridges.Load(data.SessionID); ok {
		if b, ok := bridge.(*TerminalBridge); ok {
			if conn == nil || !b.MatchesAgent(conn.AssetID) {
				return
			}
			reason := strings.TrimSpace(data.Reason)
			reasonLog := shared.StreamTraceLogValue(reason)
			log.Printf("terminal-agent: agent_reported_closed session=%s target=%s trace=%s reason=%s", b.SessionID, b.Target, shared.StreamTraceLogValue(b.TraceID), reasonLog)
			b.CloseWithReason(reason)
		}
	}
}

func SanitizeAgentStreamReason(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return "unknown"
	}
	if len(value) > 64 {
		value = value[:64]
	}

	var b strings.Builder
	b.Grow(len(value))
	underscorePending := false
	for _, ch := range value {
		switch {
		case ch >= 'a' && ch <= 'z':
			if underscorePending && b.Len() > 0 {
				b.WriteByte('_')
			}
			underscorePending = false
			b.WriteRune(ch)
		case ch >= '0' && ch <= '9':
			if underscorePending && b.Len() > 0 {
				b.WriteByte('_')
			}
			underscorePending = false
			b.WriteRune(ch)
		case ch == '-' || ch == '_' || ch == '.':
			if underscorePending && b.Len() > 0 {
				b.WriteByte('_')
				underscorePending = false
			}
			b.WriteRune(ch)
		default:
			underscorePending = true
		}
	}

	result := strings.Trim(b.String(), "_.-")
	if result == "" {
		return "unknown"
	}
	return result
}

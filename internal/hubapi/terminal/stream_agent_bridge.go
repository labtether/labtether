package terminal

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

func SendTerminalClose(agentConn *agentmgr.AgentConn, sessionID string) {
	if agentConn == nil || sessionID == "" {
		return
	}
	closeData, _ := json.Marshal(agentmgr.TerminalCloseData{SessionID: sessionID})
	_ = agentConn.Send(agentmgr.Message{
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

// HandleAgentTerminalStream bridges the browser WebSocket to an agent PTY session.
func (d *Deps) HandleAgentTerminalStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	traceID := shared.BrowserStreamTraceID(r)
	traceLog := shared.StreamTraceLogValue(traceID)
	logContext := fmt.Sprintf("session=%s target=%s trace=%s", session.ID, session.Target, traceLog)

	agentConn, ok := d.AgentMgr.Get(session.Target)
	if !ok {
		log.Printf("terminal-agent: stream_setup_failed reason=agent_disconnected %s", logContext) // #nosec G706 -- Log context is composed from bounded session/runtime identifiers.
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	wsConn, err := d.TerminalWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("terminal-agent: upgrade_failed %s err=%v", logContext, err) // #nosec G706 -- Log fields are bounded session/runtime identifiers and local upgrade errors.
		return
	}
	defer wsConn.Close()
	wsConn.SetReadLimit(maxTerminalInputReadBytes)
	_ = WriteTerminalStatus(wsConn, "agent_connecting", "Connecting to agent terminal...", 0, 0, 0)
	connectStart := time.Now()

	cols, rows := ParseTerminalSize(r.URL.Query().Get("cols"), r.URL.Query().Get("rows"))

	// Probe agent for tmux availability before starting the terminal.
	probeResult := d.ProbeAgentTmux(agentConn)

	// Set up output channel for this terminal session.
	outputCh := make(chan []byte, 256)
	closedCh := make(chan struct{})
	bridgeState := &TerminalBridge{
		OutputCh:        outputCh,
		ClosedCh:        closedCh,
		ExpectedAgentID: agentConn.AssetID,
		SessionID:       session.ID,
		Target:          session.Target,
		TraceID:         traceID,
	}
	d.TerminalBridges.Store(session.ID, bridgeState)
	startSent := false
	endReason := "unknown"
	var endErr error
	defer func() {
		if endReason == "unknown" {
			closeReason := SanitizeAgentStreamReason(bridgeState.CloseReasonOr("stream_closed"))
			endReason = "stream_finalized_" + closeReason
		}
		if endErr != nil {
			log.Printf("terminal-agent: stream_ended reason=%s %s elapsed=%s err=%v", endReason, logContext, time.Since(connectStart).Round(time.Millisecond), endErr) // #nosec G706 -- Stream reason/context values are sanitized bounded runtime metadata.
		} else {
			log.Printf("terminal-agent: stream_ended reason=%s %s elapsed=%s", endReason, logContext, time.Since(connectStart).Round(time.Millisecond)) // #nosec G706 -- Stream reason/context values are sanitized bounded runtime metadata.
		}
		d.FinalizeAgentTerminalSession(session.ID, bridgeState, agentConn, startSent, nil)
	}()

	// Build terminal start request, with tmux if available.
	startReq := agentmgr.TerminalStartData{
		SessionID: session.ID,
		Cols:      cols,
		Rows:      rows,
	}
	if probeResult.HasTmux {
		tmuxSessionName := strings.TrimSpace(session.TmuxSessionName)
		if tmuxSessionName == "" {
			tmuxSessionName = "lt-" + session.ID
			if len(tmuxSessionName) > 11 { // "lt-" + 8 chars
				tmuxSessionName = tmuxSessionName[:11]
			}
		}
		startReq.UseTmux = true
		startReq.TmuxSession = tmuxSessionName
		log.Printf("terminal: tmux available on %s, using session %q", session.Target, tmuxSessionName)
	}

	// Send terminal.start to agent.
	startData, _ := json.Marshal(startReq)
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgTerminalStart,
		ID:   session.ID,
		Data: startData,
	}); err != nil {
		endReason = "agent_start_send_failed"
		endErr = err
		log.Printf("terminal-agent: stream_setup_failed reason=agent_start_send_failed %s err=%v", logContext, err) // #nosec G706 -- Log fields are bounded session/runtime identifiers and local bridge errors.
		_ = WriteTerminalError(wsConn, "agent_start_failed", "Failed to start terminal on agent")
		return
	}
	startSent = true
	_ = WriteTerminalStatus(wsConn, "agent_starting_shell", "Starting remote shell...", 0, 0, 0)
	var writeMu sync.Mutex

	// Wait for terminal.started with timeout.
	select {
	case <-closedCh:
		closeReason := SanitizeAgentStreamReason(bridgeState.CloseReasonOr("agent_closed_before_ready"))
		endReason = "agent_closed_before_ready_" + closeReason
		endErr = fmt.Errorf("agent closed before terminal started: %s", bridgeState.CloseReasonOr("agent_closed_before_ready"))
		log.Printf("terminal-agent: stream_setup_failed reason=%s %s", endReason, logContext) // #nosec G706 -- Reason and context are bounded runtime values.
		_ = WriteTerminalError(wsConn, "agent_closed", "Agent terminal session closed unexpectedly")
		return
	case <-time.After(10 * time.Second):
		endReason = "agent_start_timeout"
		log.Printf("terminal-agent: stream_setup_failed reason=agent_start_timeout %s", logContext) // #nosec G706 -- Log context is composed from bounded session/runtime identifiers.
		_ = WriteTerminalError(wsConn, "agent_start_timeout", "Agent terminal start timed out")
		return
	case data := <-outputCh:
		// First message is either started confirmation or output data.
		if data != nil {
			log.Printf("terminal-agent: stream_connected %s cols=%d rows=%d ready=output-first", logContext, cols, rows) // #nosec G706 -- Log context is composed from bounded session/runtime identifiers.
			_ = WriteTerminalReady(wsConn, "Terminal connected", 0)
			stopKeepalive := shared.StartBrowserWebSocketKeepalive(wsConn, &writeMu, "terminal-agent:"+session.ID)
			defer stopKeepalive()
			go func() {
				writeMu.Lock()
				_ = wsConn.WriteMessage(websocket.BinaryMessage, data)
				writeMu.Unlock()
				d.BridgeAgentOutput(wsConn, outputCh, closedCh, &writeMu, bridgeState, logContext)
			}()
			endReason, endErr = d.BridgeAgentInput(wsConn, agentConn, session.ID, closedCh, bridgeState)
			return
		}
		// nil = started confirmation, continue normally.
	}

	log.Printf("terminal: agent session started for %s on %s", session.ID, session.Target)    // #nosec G706 -- Session and target IDs are hub-controlled runtime identifiers.
	log.Printf("terminal-agent: stream_connected %s cols=%d rows=%d", logContext, cols, rows) // #nosec G706 -- Log context is composed from bounded session/runtime identifiers.
	_ = WriteTerminalReady(wsConn, "Terminal connected", 0)

	stopKeepalive := shared.StartBrowserWebSocketKeepalive(wsConn, &writeMu, "terminal-agent:"+session.ID)
	defer stopKeepalive()

	// Bridge: agent output -> browser
	go d.BridgeAgentOutput(wsConn, outputCh, closedCh, &writeMu, bridgeState, logContext)

	// Bridge: browser input -> agent
	endReason, endErr = d.BridgeAgentInput(wsConn, agentConn, session.ID, closedCh, bridgeState)
}

// BridgeAgentOutput reads terminal output from the agent channel and writes to browser WebSocket.
func (d *Deps) BridgeAgentOutput(wsConn *websocket.Conn, outputCh <-chan []byte, closedCh <-chan struct{}, writeMu *sync.Mutex, bridgeState *TerminalBridge, logContext string) {
	for {
		select {
		case data, ok := <-outputCh:
			if !ok {
				log.Printf("terminal-agent: stream_runtime_event reason=agent_output_channel_closed %s", logContext) // #nosec G706 -- Log context is composed from bounded session/runtime identifiers.
				return
			}
			if data == nil {
				continue // skip markers
			}
			writeMu.Lock()
			_ = wsConn.SetWriteDeadline(time.Now().Add(TerminalStreamWriteDeadline))
			err := wsConn.WriteMessage(websocket.BinaryMessage, data)
			writeMu.Unlock()
			if err != nil {
				log.Printf("terminal-agent: stream_runtime_event reason=browser_ws_write_failed %s err=%v", logContext, err) // #nosec G706 -- Log context is composed from bounded session/runtime identifiers.
				return
			}
		case <-closedCh:
			closeReason := "-"
			if bridgeState != nil {
				closeReason = shared.StreamTraceLogValue(strings.TrimSpace(bridgeState.CloseReasonOr("agent_closed")))
			}
			// #nosec G706 -- log values are sanitized and emitted only as structured data fields.
			slog.Info("terminal-agent: stream_runtime_event", "reason", "agent_stream_closed", "log_context", logContext, "close_reason", closeReason)
			return
		}
	}
}

// BridgeAgentInput reads from browser WebSocket and sends to agent.
func (d *Deps) BridgeAgentInput(wsConn *websocket.Conn, agentConn *agentmgr.AgentConn, sessionID string, closedCh <-chan struct{}, bridgeState *TerminalBridge) (endReason string, endErr error) {
	endReason = "unknown"

	stopCloser := make(chan struct{})
	defer close(stopCloser)
	go func() {
		select {
		case <-closedCh:
			_ = wsConn.SetReadDeadline(time.Now())
			_ = wsConn.Close()
		case <-stopCloser:
		}
	}()

	defer func() {
		if recovered := recover(); recovered != nil {
			endReason = "browser_input_bridge_panic"
			endErr = fmt.Errorf("browser input bridge panic: %v", recovered)
			log.Printf("terminal: browser input bridge panic for %s: %v", sessionID, recovered)
		}
	}()

	for {
		messageType, payload, err := wsConn.ReadMessage()
		if err != nil {
			select {
			case <-closedCh:
				closeReason := "agent_closed"
				if bridgeState != nil {
					closeReason = SanitizeAgentStreamReason(bridgeState.CloseReasonOr("agent_closed"))
				}
				return "agent_stream_closed_" + closeReason, nil
			default:
			}

			switch {
			case websocket.IsCloseError(err, websocket.CloseNormalClosure):
				return "browser_ws_closed_normal", nil
			case websocket.IsCloseError(err, websocket.CloseGoingAway):
				return "browser_ws_closed_going_away", nil
			default:
				var closeErr *websocket.CloseError
				switch {
				case errors.As(err, &closeErr):
					return fmt.Sprintf("browser_ws_closed_code_%d", closeErr.Code), err
				default:
					var netErr net.Error
					if errors.As(err, &netErr) && netErr.Timeout() {
						return "browser_ws_read_timeout", err
					}
					return "browser_ws_read_error", err
				}
			}
		}
		_ = shared.TouchBrowserWebSocketReadDeadline(wsConn)
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}

		// Check for JSON control messages (resize, ping).
		if IsControlMessage(payload) {
			msg := TerminalControlMessage{}
			if json.Unmarshal(payload, &msg) == nil {
				switch strings.ToLower(msg.Type) {
				case "resize":
					resizeData, _ := json.Marshal(agentmgr.TerminalResizeData{
						SessionID: sessionID,
						Cols:      msg.Cols,
						Rows:      msg.Rows,
					})
					if err := agentConn.Send(agentmgr.Message{
						Type: agentmgr.MsgTerminalResize,
						ID:   sessionID,
						Data: resizeData,
					}); err != nil {
						return "agent_resize_send_failed", err
					}
					continue
				case "ping":
					continue
				case "input":
					payload = []byte(msg.Data)
				}
			}
		}

		// Send terminal input to agent.
		encoded := base64.StdEncoding.EncodeToString(payload)
		inputData, _ := json.Marshal(agentmgr.TerminalDataPayload{
			SessionID: sessionID,
			Data:      encoded,
		})
		if err := agentConn.Send(agentmgr.Message{
			Type: agentmgr.MsgTerminalData,
			ID:   sessionID,
			Data: inputData,
		}); err != nil {
			return "agent_data_send_failed", err
		}
	}
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

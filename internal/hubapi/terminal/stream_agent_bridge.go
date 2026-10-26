package terminal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

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
	if strings.TrimSpace(session.PersistentSessionID) != "" || strings.TrimSpace(session.TmuxSessionName) != "" {
		if err := d.ValidatePersistentAgentTmux(agentConn); err != nil {
			log.Printf("terminal-agent: stream_setup_failed reason=persistent_tmux_unavailable %s err=%v", logContext, err) // #nosec G706 -- Log fields are bounded runtime identifiers and local capability errors.
			servicehttp.WriteError(w, http.StatusConflict, err.Error())
			return
		}
	}

	wsConn, err := shared.UpgradeWebSocket(&d.TerminalWebSocketUpgrader, w, r, nil)
	if err != nil {
		log.Printf("terminal-agent: upgrade_failed %s err=%v", logContext, err) // #nosec G706 -- Log fields are bounded session/runtime identifiers and local upgrade errors.
		return
	}
	wsConn.SetReadLimit(maxTerminalInputReadBytes)
	defer wsConn.Close()

	persistentSessionID := strings.TrimSpace(session.PersistentSessionID)
	connectStart := time.Now()

	// Set up output channel for this terminal session.
	outputCh := make(chan []byte, 256)
	closedCh := make(chan struct{})
	wireSessionID := "terminal-stream:" + shared.GenerateRequestID()
	bridgeState := &TerminalBridge{
		OutputCh:            outputCh,
		ClosedCh:            closedCh,
		ExpectedAgentID:     agentConn.AssetID,
		AgentConn:           agentConn,
		PersistentSessionID: persistentSessionID,
		SessionID:           session.ID,
		WireSessionID:       wireSessionID,
		Target:              session.Target,
		TraceID:             traceID,
		RequireTmux:         strings.TrimSpace(session.PersistentSessionID) != "" || strings.TrimSpace(session.TmuxSessionName) != "",
	}
	var flushCancel context.CancelFunc
	var flushDone chan struct{}
	startSent := false
	published := false
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
		if flushCancel != nil {
			flushCancel()
			<-flushDone
		}
		if published {
			d.detachPersistentTerminalStream(session, wsConn)
		}
	}()

	if !d.publishAgentTerminalStream(r.Context(), bridgeState, persistentSessionID, wsConn) {
		endReason = "agent_disconnected_before_publish"
		bridgeState.CloseWithReason("agent_disconnected")
		_ = WriteTerminalError(wsConn, "agent_disconnected", "Agent disconnected while opening terminal")
		return
	}
	published = true
	_ = WriteTerminalStatus(wsConn, "agent_connecting", "Connecting to agent terminal...", 0, 0, 0)
	cols, rows := ParseTerminalSize(r.URL.Query().Get("cols"), r.URL.Query().Get("rows"))
	probeResult := d.ProbeAgentTmux(agentConn)

	if persistentSessionID != "" && d.TerminalInMemStore != nil {
		bridgeState.Scrollback = d.TerminalInMemStore.GetOrCreateScrollbackBuffer(persistentSessionID)
		if d.TerminalScrollbackStore != nil {
			if stored, loadErr := d.TerminalScrollbackStore.GetScrollback(persistentSessionID); loadErr == nil {
				bridgeState.Scrollback.SeedIfEmpty(stored)
			}
			var flushCtx context.Context
			flushCtx, flushCancel = context.WithCancel(context.Background())
			flushDone = make(chan struct{})
			go func() {
				defer close(flushDone)
				d.scrollbackFlushLoop(flushCtx, persistentSessionID, bridgeState.Scrollback)
			}()
		}
	}

	// Snapshot only the history that predates this wire stream. Every byte from
	// the new PTY stays owned by the output queue, including a fast shell's exit.
	replayOutput := d.snapshotAgentScrollback(bridgeState)

	// Build terminal start request, with tmux if available.
	startReq := agentmgr.TerminalStartData{
		SessionID: wireSessionID,
		Cols:      cols,
		Rows:      rows,
	}
	if probeResult.HasTmux {
		tmuxSessionName := strings.TrimSpace(session.TmuxSessionName)
		if tmuxSessionName == "" {
			tmuxSessionName = DefaultTerminalTmuxSessionName(session.ID)
		}
		startReq.UseTmux = true
		startReq.TmuxSession = tmuxSessionName
		log.Printf("terminal: tmux available on %s, using session %q", session.Target, tmuxSessionName)
	}

	// Do not start a PTY after another agent or SSH attachment evicts us.
	if !d.ownsAgentTerminalStream(bridgeState, persistentSessionID, wsConn) {
		endReason = "superseded_before_start"
		return
	}
	select {
	case <-closedCh:
		endReason = "superseded_before_start"
		return
	default:
	}
	startData, _ := json.Marshal(startReq)
	var startErr error
	if !shared.RunIfSessionStreamOpen(r.Context(), func() {
		startErr = d.startAgentTerminalStream(bridgeState, persistentSessionID, wsConn, agentmgr.Message{
			Type: agentmgr.MsgTerminalStart,
			ID:   wireSessionID,
			Data: startData,
		})
	}) {
		endReason = "session_request_cancelled"
		return
	}
	if err := startErr; err != nil {
		endReason = "agent_start_send_failed"
		endErr = err
		log.Printf("terminal-agent: stream_setup_failed reason=agent_start_send_failed %s err=%v", logContext, err) // #nosec G706 -- Log fields are bounded session/runtime identifiers and local bridge errors.
		_ = WriteTerminalError(wsConn, "agent_start_failed", "Failed to start terminal on agent")
		return
	}
	startSent = true
	_ = WriteTerminalStatus(wsConn, "agent_starting_shell", "Starting remote shell...", 0, 0, 0)
	var writeMu sync.Mutex

	// The queue can contain the final output before startup observes closure.
	// Keep that prefix for the output pump instead of abandoning it on exit.
	var firstOutput []byte
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-r.Context().Done():
		endReason = "session_request_cancelled"
		return
	case <-closedCh:
		select {
		case firstOutput = <-outputCh:
		default:
			rawCloseReason := bridgeState.CloseReasonOr("agent_closed_before_ready")
			closeReason := SanitizeAgentStreamReason(rawCloseReason)
			endReason = "agent_closed_before_ready_" + closeReason
			endErr = fmt.Errorf("agent closed before terminal started: %s", rawCloseReason)
			if closeReason == "tmux_unavailable" {
				_ = WriteTerminalError(wsConn, "tmux_unavailable", "Persistent terminal requires tmux, but the agent started a non-persistent shell")
			} else {
				_ = WriteTerminalError(wsConn, "agent_closed", "Agent terminal session closed unexpectedly")
			}
			return
		}
	case <-timer.C:
		endReason = "agent_start_timeout"
		log.Printf("terminal-agent: stream_setup_failed reason=agent_start_timeout %s", logContext) // #nosec G706 -- Log context contains bounded runtime values.
		_ = WriteTerminalError(wsConn, "agent_start_timeout", "Agent terminal start timed out")
		return
	case firstOutput = <-outputCh:
	}

	log.Printf("terminal-agent: stream_connected %s cols=%d rows=%d", logContext, cols, rows) // #nosec G706 -- Context is composed from bounded session identifiers.
	if err := writeTerminalOutput(wsConn, &writeMu, replayOutput); err != nil {
		endReason = "scrollback_replay_failed"
		endErr = err
		return
	}
	_ = WriteTerminalReady(wsConn, "Terminal connected", 0)
	stopKeepalive := shared.StartBrowserWebSocketKeepalive(wsConn, &writeMu, "terminal-agent:"+session.ID)
	defer stopKeepalive()
	endReason, endErr = d.bridgeAgentTerminalIO(wsConn, agentConn, bridgeState, firstOutput, &writeMu, logContext)
}

// DefaultTerminalTmuxSessionName returns a stable, session-isolated tmux name.
// Session IDs share a long generated prefix, so truncating the raw ID would
// attach otherwise unrelated browser sessions to the same remote shell.
func DefaultTerminalTmuxSessionName(sessionID string) string {
	return terminal.TmuxSessionNameForID(sessionID)
}

// BridgeAgentInput reads from browser WebSocket and sends to agent.
func (d *Deps) BridgeAgentInput(wsConn *websocket.Conn, agentConn *agentmgr.AgentConn, sessionID string, closedCh <-chan struct{}, bridgeState *TerminalBridge) (endReason string, endErr error) {
	endReason = "unknown"

	stopCloser := make(chan struct{})
	defer close(stopCloser)
	// Live streams have an output owner that drains final data before closing
	// the browser. Keep direct input-only callers' existing close behavior.
	if bridgeState == nil || bridgeState.outputDone == nil {
		go func() {
			select {
			case <-closedCh:
				_ = wsConn.SetReadDeadline(time.Now())
				_ = wsConn.Close()
			case <-stopCloser:
			}
		}()
	}

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
		select {
		case <-closedCh:
			return "agent_stream_closed_" + SanitizeAgentStreamReason(bridgeState.CloseReasonOr("agent_closed")), nil
		default:
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

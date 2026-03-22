package desktop

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

// DesktopDebugEnabled returns whether desktop debug logging is on.
var DesktopDebugEnabled = sync.OnceValue(func() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("LABTETHER_DESKTOP_DEBUG")), "true")
})

// MaxWebSocketCloseReasonBytes is the maximum bytes for a WS close reason.
const MaxWebSocketCloseReasonBytes = shared.MaxWebSocketCloseReasonBytes

// NormalizeWebSocketCloseReason truncates a close reason to protocol limits.
func NormalizeWebSocketCloseReason(reason string) string {
	return shared.NormalizeWebSocketCloseReason(reason)
}

// SafeWriteClose writes a WS close frame with a safe-length reason.
func SafeWriteClose(wsConn *websocket.Conn, code int, reason string) {
	shared.SafeWriteClose(wsConn, code, reason)
}

// SendDesktopClose sends a desktop.close message to the agent.
func SendDesktopClose(agentConn *agentmgr.AgentConn, sessionID string) {
	if agentConn == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	closeData, _ := json.Marshal(agentmgr.DesktopCloseData{SessionID: sessionID})
	_ = agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgDesktopClose,
		ID:   sessionID,
		Data: closeData,
	})
}

// FinalizeAgentDesktopSession cleans up a desktop bridge after the session ends.
func (d *Deps) FinalizeAgentDesktopSession(
	sessionID string,
	bridgeState *DesktopBridge,
	agentConn *agentmgr.AgentConn,
	startSent bool,
	closeFn func(*agentmgr.AgentConn, string),
) {
	d.DesktopBridges.Delete(sessionID)
	if bridgeState != nil {
		bridgeState.StopRecordingLocked(d.StopRecording)
		bridgeState.Close()
	}
	if startSent {
		if closeFn == nil {
			closeFn = SendDesktopClose
		}
		closeFn(agentConn, sessionID)
	}
}

// CloseDesktopBridgesForAsset closes all desktop bridges matching the given asset.
func (d *Deps) CloseDesktopBridgesForAsset(assetID string) {
	trimmedAssetID := strings.TrimSpace(assetID)
	if trimmedAssetID == "" {
		return
	}
	d.DesktopBridges.Range(func(_ any, value any) bool {
		bridge, ok := value.(*DesktopBridge)
		if !ok {
			return true
		}
		if bridge.MatchesAgent(trimmedAssetID) {
			bridge.Close()
		}
		return true
	})
}

// HandleAgentDesktopStream bridges the browser WebSocket to an agent desktop session.
func (d *Deps) HandleAgentDesktopStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	traceID := d.BrowserStreamTraceID(r)
	traceLog := d.StreamTraceLogValue(traceID)

	agentConn, ok := d.AgentMgr.Get(session.Target)
	if !ok {
		log.Printf("desktop-agent: stream_setup_failed reason=agent_disconnected session=%s target=%s trace=%s", session.ID, session.Target, traceLog) // #nosec G706 -- Session, target, and trace IDs are hub-controlled runtime identifiers.
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	wsConn, err := d.TerminalWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("desktop-agent: upgrade_failed session=%s target=%s trace=%s err=%v", session.ID, session.Target, traceLog, err) // #nosec G706 -- Session, target, and trace IDs are hub-controlled runtime identifiers.
		return
	}
	defer wsConn.Close()
	wsConn.SetReadLimit(d.MaxDesktopInputReadBytes)
	connectStart := time.Now()

	opts := d.GetDesktopSessionOptions(session.ID)
	quality := strings.TrimSpace(r.URL.Query().Get("quality"))
	if quality == "" {
		quality = opts.Quality
	}
	display := strings.TrimSpace(r.URL.Query().Get("display"))
	if display == "" {
		display = opts.Display
	}
	protocol := d.ResolveDesktopProtocol(session, r)
	logContext := fmt.Sprintf("session=%s target=%s trace=%s protocol=%s", session.ID, session.Target, traceLog, d.StreamTraceLogValue(protocol))

	// Set up output channel for this desktop session.
	outputCh := make(chan []byte, 1024)
	closedCh := make(chan struct{})
	bridgeState := &DesktopBridge{
		OutputCh:        outputCh,
		AudioCh:         make(chan DesktopAudioOutbound, 256),
		ClosedCh:        closedCh,
		ExpectedAgentID: session.Target,
		SessionID:       session.ID,
		Target:          session.Target,
		TraceID:         traceID,
	}
	if opts.Record || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("record")), "1") {
		recording, err := d.StartRecording(session.ID, session.Target, session.ActorID, protocol)
		if err != nil {
			log.Printf("recording: failed to start for session=%s: %v", session.ID, err)
		} else {
			bridgeState.SetRecording(recording)
		}
	}
	d.DesktopBridges.Store(session.ID, bridgeState)
	startSent := false
	endReason := "unknown"
	var endErr error
	defer func() {
		if endReason == "unknown" {
			closeReason := strings.TrimSpace(bridgeState.CloseReason())
			if closeReason == "" {
				closeReason = "stream_closed"
			}
			endReason = "stream_finalized_" + d.SanitizeAgentStreamReason(closeReason)
		}
		if endErr != nil {
			log.Printf("desktop-agent: stream_ended reason=%s %s elapsed=%s err=%v", endReason, logContext, time.Since(connectStart).Round(time.Millisecond), endErr) // #nosec G706 -- Stream reason/context values are sanitized bounded runtime metadata.
		} else {
			log.Printf("desktop-agent: stream_ended reason=%s %s elapsed=%s", endReason, logContext, time.Since(connectStart).Round(time.Millisecond)) // #nosec G706 -- Stream reason/context values are sanitized bounded runtime metadata.
		}
		d.FinalizeAgentDesktopSession(session.ID, bridgeState, agentConn, startSent, nil)
	}()

	// Send desktop.start to agent.
	startData, _ := json.Marshal(agentmgr.DesktopStartData{
		SessionID:   session.ID,
		Quality:     quality,
		Display:     display,
		VNCPassword: opts.VNCPassword,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgDesktopStart,
		ID:   session.ID,
		Data: startData,
	}); err != nil {
		endReason = "agent_start_send_failed"
		endErr = err
		log.Printf("desktop-agent: stream_setup_failed reason=agent_start_send_failed %s err=%v", logContext, err) // #nosec G706 -- Log fields are bounded session/runtime identifiers and local bridge errors.
		SafeWriteClose(wsConn, websocket.CloseTryAgainLater, "failed to start desktop on agent")
		return
	}
	startSent = true
	var writeMu sync.Mutex

	// If this VNC session is a fallback from a WebRTC request, notify the browser.
	if fallbackReason := strings.TrimSpace(opts.FallbackReason); fallbackReason != "" {
		notice, _ := json.Marshal(map[string]string{
			"type":   "protocol_fallback",
			"from":   "webrtc",
			"to":     "vnc",
			"reason": fallbackReason,
		})
		writeMu.Lock()
		_ = wsConn.WriteMessage(websocket.TextMessage, notice)
		writeMu.Unlock()
	}

	// Wait for desktop.started with timeout.
	select {
	case <-closedCh:
		reason := strings.TrimSpace(bridgeState.CloseReason())
		if reason == "" {
			reason = "agent_closed_before_ready"
		}
		endReason = "agent_closed_before_ready_" + d.SanitizeAgentStreamReason(reason)
		log.Printf("desktop-agent: stream_setup_failed reason=%s %s", endReason, logContext) // #nosec G706 -- Reason and context are bounded runtime values.
		SafeWriteClose(wsConn, websocket.CloseTryAgainLater, "agent desktop session closed unexpectedly")
		return
	case <-time.After(15 * time.Second):
		endReason = "agent_start_timeout"
		log.Printf("desktop-agent: stream_setup_failed reason=agent_start_timeout %s", logContext) // #nosec G706 -- Log context is composed from bounded session/runtime identifiers.
		SafeWriteClose(wsConn, websocket.CloseTryAgainLater, "agent desktop start timed out")
		return
	case data := <-outputCh:
		if data != nil {
			// Got VNC data before started marker — write it through.
			log.Printf("desktop-agent: stream_connected %s ready=output-first", logContext) // #nosec G706 -- Log context is composed from bounded session/runtime identifiers.
			stopKeepalive := d.StartBrowserWSKeepalive(wsConn, &writeMu, "desktop-agent:"+session.ID)
			defer stopKeepalive()
			go func() {
				writeMu.Lock()
				_ = wsConn.WriteMessage(websocket.BinaryMessage, data)
				writeMu.Unlock()
				d.BridgeDesktopOutput(wsConn, outputCh, closedCh, &writeMu, bridgeState, logContext)
			}()
			endReason, endErr = d.BridgeDesktopInput(wsConn, agentConn, session.ID, closedCh, bridgeState)
			return
		}
		// nil = started confirmation, continue normally.
	}

	log.Printf("desktop: agent session started for %s on %s", session.ID, session.Target) // #nosec G706 -- Session and target IDs are hub-controlled runtime identifiers.
	log.Printf("desktop-agent: stream_connected %s", logContext)                          // #nosec G706 -- Log context is composed from bounded session/runtime identifiers.

	stopKeepalive := d.StartBrowserWSKeepalive(wsConn, &writeMu, "desktop-agent:"+session.ID)
	defer stopKeepalive()

	// Bridge: agent output → browser
	go d.BridgeDesktopOutput(wsConn, outputCh, closedCh, &writeMu, bridgeState, logContext)

	// Bridge: browser input → agent
	endReason, endErr = d.BridgeDesktopInput(wsConn, agentConn, session.ID, closedCh, bridgeState)

	if reason := bridgeState.CloseReason(); reason != "" {
		if endReason == "unknown" {
			endReason = "agent_reported_close_" + d.SanitizeAgentStreamReason(reason)
		}
		SafeWriteClose(wsConn, websocket.CloseNormalClosure, reason)
	}
}

// BridgeDesktopOutput reads VNC output from the agent channel and writes to browser WebSocket.
func (d *Deps) BridgeDesktopOutput(
	wsConn *websocket.Conn,
	outputCh <-chan []byte,
	closedCh <-chan struct{},
	writeMu *sync.Mutex,
	bridge *DesktopBridge,
	logContext string,
) {
	for {
		select {
		case data, ok := <-outputCh:
			if !ok {
				log.Printf("desktop-agent: stream_runtime_event reason=agent_output_channel_closed %s", logContext)
				return
			}
			if data == nil {
				continue
			}
			if bridge != nil {
				if rec := bridge.CurrentRecording(); rec != nil {
					rec.WriteFrame(data)
				}
			}
			writeMu.Lock()
			_ = wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := wsConn.WriteMessage(websocket.BinaryMessage, data)
			writeMu.Unlock()
			if err != nil {
				log.Printf("desktop-agent: stream_runtime_event reason=browser_ws_write_failed %s err=%v", logContext, err)
				if bridge != nil {
					bridge.SetReason("desktop websocket write failed")
					bridge.Close()
				}
				_ = wsConn.Close()
				return
			}
		case <-closedCh:
			closeReason := "-"
			if bridge != nil {
				closeReason = d.StreamTraceLogValue(strings.TrimSpace(bridge.CloseReason()))
			}
			// #nosec G706 -- log values are sanitized and emitted only as structured data fields.
			slog.Info("desktop-agent: stream_runtime_event", "reason", "agent_stream_closed", "log_context", logContext, "close_reason", closeReason)
			return
		}
	}
}

// BridgeDesktopInput reads from browser WebSocket and sends VNC data to agent.
func (d *Deps) BridgeDesktopInput(wsConn *websocket.Conn, agentConn *agentmgr.AgentConn, sessionID string, closedCh <-chan struct{}, bridgeState *DesktopBridge) (endReason string, endErr error) {
	endReason = "unknown"
	if agentConn == nil {
		log.Printf("desktop: bridgeDesktopInput missing agent connection for %s; dropping browser input until session close", sessionID)
	}

	// A read timeout on Gorilla websocket marks the connection as failed; repeated
	// reads after that can panic. Keep reads blocking and force-unblock by closing
	// the socket when the session closes.
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
			log.Printf("desktop: browser input bridge panic for %s: %v", sessionID, recovered)
		}
	}()

	for {
		messageType, payload, err := wsConn.ReadMessage()
		if err != nil {
			select {
			case <-closedCh:
				closeReason := "agent_closed"
				if bridgeState != nil {
					closeReason = strings.TrimSpace(bridgeState.CloseReason())
					if closeReason == "" {
						closeReason = "agent_closed"
					}
				}
				return "agent_stream_closed_" + d.SanitizeAgentStreamReason(closeReason), nil
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
		_ = d.TouchBrowserWSReadDeadline(wsConn)
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		if agentConn == nil {
			continue
		}

		// Send VNC input to agent as base64-encoded data.
		encoded := base64.StdEncoding.EncodeToString(payload)
		inputData, _ := json.Marshal(agentmgr.DesktopDataPayload{
			SessionID: sessionID,
			Data:      encoded,
		})
		if err := agentConn.Send(agentmgr.Message{
			Type: agentmgr.MsgDesktopData,
			ID:   sessionID,
			Data: inputData,
		}); err != nil {
			log.Printf("desktop: failed to send input to agent for %s: %v", sessionID, err)
			return "agent_input_send_failed", err
		}
	}
}

// ProcessAgentDesktopStarted handles desktop.started from agent.
func (d *Deps) ProcessAgentDesktopStarted(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.DesktopStartedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	if raw, ok := d.DesktopBridges.Load(data.SessionID); ok {
		b, ok := raw.(*DesktopBridge)
		if !ok || b == nil {
			return
		}
		assetID := ""
		if conn != nil {
			assetID = conn.AssetID
		}
		if !b.MatchesAgent(assetID) {
			return
		}
		log.Printf("desktop-agent: agent_reported_started session=%s target=%s trace=%s", b.SessionID, b.Target, d.StreamTraceLogValue(b.TraceID))
		b.TrySendOutput(nil) // nil = started marker
	}
}

// ProcessAgentDesktopData handles desktop.data (VNC output) from agent.
func (d *Deps) ProcessAgentDesktopData(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var payload agentmgr.DesktopDataPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil || len(decoded) == 0 {
		return
	}

	if DesktopDebugEnabled() {
		log.Printf("desktop-debug: agent_data session=%s bytes=%d", payload.SessionID, len(decoded))
	}

	if raw, ok := d.DesktopBridges.Load(payload.SessionID); ok {
		b, ok := raw.(*DesktopBridge)
		if !ok || b == nil {
			return
		}
		assetID := ""
		if conn != nil {
			assetID = conn.AssetID
		}
		if !b.MatchesAgent(assetID) {
			return
		}
		b.TrySendOutput(decoded)
	}
}

// ProcessAgentDesktopClosed handles desktop.closed from agent.
func (d *Deps) ProcessAgentDesktopClosed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.DesktopCloseData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	if raw, ok := d.DesktopBridges.Load(data.SessionID); ok {
		b, ok := raw.(*DesktopBridge)
		if !ok || b == nil {
			return
		}
		assetID := ""
		if conn != nil {
			assetID = conn.AssetID
		}
		if !b.MatchesAgent(assetID) {
			return
		}
		reason := strings.TrimSpace(data.Reason)
		log.Printf("desktop-agent: agent_reported_closed session=%s target=%s trace=%s reason=%s", b.SessionID, b.Target, d.StreamTraceLogValue(b.TraceID), d.StreamTraceLogValue(reason))
		b.SetReason(data.Reason)
		b.Close()
	}
}

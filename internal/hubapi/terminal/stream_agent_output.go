package terminal

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

const agentTerminalFinalOutputTimeout = 2 * time.Second

func (d *Deps) bridgeAgentTerminalIO(wsConn *websocket.Conn, agentConn *agentmgr.AgentConn, bridge *TerminalBridge, firstOutput []byte, writeMu *sync.Mutex, logContext string) (string, error) {
	bridge.outputDone = make(chan struct{})
	go func() {
		defer close(bridge.outputDone)
		d.bridgeAgentOutputWithInitial(wsConn, bridge.OutputCh, bridge.ClosedCh, writeMu, bridge, logContext, firstOutput)
	}()
	defer func() {
		bridge.Close()
		_ = wsConn.Close()
		<-bridge.outputDone
	}()
	reason, err := d.BridgeAgentInput(wsConn, agentConn, bridge.WireSessionID, bridge.ClosedCh, bridge)
	select {
	case <-bridge.ClosedCh:
		<-bridge.outputDone
	default:
	}
	return reason, err
}

// BridgeAgentOutput forwards output and owns browser closure. This unblocks an
// idle input read after shell exit and after an output transport failure.
func (d *Deps) BridgeAgentOutput(wsConn *websocket.Conn, outputCh <-chan []byte, closedCh <-chan struct{}, writeMu *sync.Mutex, bridge *TerminalBridge, logContext string) {
	d.bridgeAgentOutputWithInitial(wsConn, outputCh, closedCh, writeMu, bridge, logContext, nil)
}

func (d *Deps) bridgeAgentOutputWithInitial(wsConn *websocket.Conn, outputCh <-chan []byte, closedCh <-chan struct{}, writeMu *sync.Mutex, bridge *TerminalBridge, logContext string, initial []byte) {
	defer wsConn.Close()
	if len(initial) != 0 && writeTerminalOutput(wsConn, writeMu, initial) != nil {
		return
	}
	for {
		select {
		case <-closedCh:
			if drainAgentTerminalOutput(wsConn, outputCh, writeMu) {
				closeAgentTerminalBrowser(wsConn, bridge.CloseReasonOr(""))
			}
			reason := shared.StreamTraceLogValue(strings.TrimSpace(bridge.CloseReasonOr("agent_closed")))
			slog.Info("terminal-agent: stream_runtime_event", "reason", "agent_stream_closed", "log_context", logContext, "close_reason", reason)
			return
		default:
		}
		select {
		case <-closedCh:
			// The next iteration gives closure priority over queued output.
			continue
		case data, ok := <-outputCh:
			if !ok || writeTerminalOutput(wsConn, writeMu, data) != nil {
				return
			}
		}
	}
}

func drainAgentTerminalOutput(wsConn *websocket.Conn, outputCh <-chan []byte, writeMu *sync.Mutex) bool {
	// Bridge closure fences further queue writes. Drain this finite prefix with
	// a single deadline, then release the browser even if its reader stalls.
	deadline := time.Now().Add(agentTerminalFinalOutputTimeout)
	for remaining := len(outputCh); remaining > 0; remaining-- {
		select {
		case data, ok := <-outputCh:
			if !ok || time.Now().After(deadline) {
				return false
			}
			if len(data) == 0 {
				continue
			}
			writeMu.Lock()
			_ = wsConn.SetWriteDeadline(deadline)
			err := wsConn.WriteMessage(websocket.BinaryMessage, data)
			writeMu.Unlock()
			if err != nil {
				return false
			}
		default:
			return true
		}
	}
	return true
}

func closeAgentTerminalBrowser(wsConn *websocket.Conn, reason string) {
	code, message := websocket.CloseInternalServerErr, "Terminal session ended"
	switch reason {
	case "", "shell exited", "persistent_session_deleted":
		code, message = websocket.CloseNormalClosure, ""
	case "session_attached_elsewhere":
		code, message = websocket.CloseNormalClosure, "Session attached elsewhere"
	case "agent_disconnected":
		code, message = websocket.CloseGoingAway, "Agent disconnected"
	case "output_overflow":
		message = "Terminal output buffer overflow"
	}
	_ = wsConn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, message), time.Now().Add(agentTerminalFinalOutputTimeout))
}

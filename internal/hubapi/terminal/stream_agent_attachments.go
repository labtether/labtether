package terminal

import (
	"context"
	"errors"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
)

// publishAgentTerminalStream registers the stream inside the logical-session
// gate and the agent lifecycle, so neither DELETE nor an agent replacement can
// miss it. Persistence and closing the previous browser happen after the
// agent gate, because both can block.
func (d *Deps) publishAgentTerminalStream(ctx context.Context, bridge *TerminalBridge, persistentID string, wsConn *websocket.Conn) bool {
	published := false
	var previous *websocket.Conn
	open := shared.RunIfSessionStreamOpen(ctx, func() {
		published = d.AgentMgr.DispatchIfCurrent(bridge.AgentConn, func() {
			previous = d.registerAgentTerminalStream(bridge, persistentID, wsConn)
		})
		if published && persistentID != "" {
			d.markPersistentTerminalStreamAttached(persistentID, wsConn)
		}
	})
	closePersistentBrowser(previous)
	return open && published
}

// Register both owners together so concurrent reconnects cannot publish the
// public bridge in one order and the persistent browser socket in another.
// It returns the replaced persistent browser for the caller to close.
func (d *Deps) registerAgentTerminalStream(bridge *TerminalBridge, persistentID string, wsConn *websocket.Conn) *websocket.Conn {
	d.agentStreamMu.Lock()
	if d.AgentMgr != nil {
		current, connected := d.AgentMgr.Get(bridge.ExpectedAgentID)
		if !connected || current != bridge.AgentConn {
			d.agentStreamMu.Unlock()
			bridge.CloseWithReason("agent_disconnected")
			return nil
		}
	}
	if persistentID != "" {
		d.closePersistentAgentStreamsLocked(persistentID)
	}
	if previous, _ := d.ActiveAgentStreams.Load(bridge.SessionID); previous != nil {
		if old, ok := previous.(*TerminalBridge); ok {
			old.CloseWithReason("session_attached_elsewhere")
		}
	}
	d.TerminalBridges.Store(bridge.WireSessionID, bridge)
	d.ActiveAgentStreams.Swap(bridge.SessionID, bridge)
	var previousSocket any
	if persistentID != "" {
		previousSocket, _ = d.ActivePersistentConns.Swap(persistentID, wsConn)
	}
	d.agentStreamMu.Unlock()
	if previousConn, ok := previousSocket.(*websocket.Conn); ok && previousConn != wsConn {
		return previousConn
	}
	return nil
}

// The caller owns agentStreamMu. Close old writers before publishing an agent
// or SSH replacement, so they cannot append to its shared scrollback snapshot.
func (d *Deps) closePersistentAgentStreamsLocked(persistentID string) {
	if d.TerminalBridges == nil {
		return
	}
	d.TerminalBridges.Range(func(_ any, value any) bool {
		if old, ok := value.(*TerminalBridge); ok && old.PersistentSessionID == persistentID {
			old.CloseWithReason("session_attached_elsewhere")
		}
		return true
	})
}

func (d *Deps) ownsAgentTerminalStream(bridge *TerminalBridge, persistentID string, wsConn *websocket.Conn) bool {
	owner, _ := d.ActiveAgentStreams.Load(bridge.SessionID)
	if owner != bridge {
		return false
	}
	if persistentID != "" {
		owner, _ = d.ActivePersistentConns.Load(persistentID)
		return owner == wsConn
	}
	return true
}

// Keep owner replacement, bridge closure, and start ordered. A browser evicted
// during startup must not launch a PTY after the replacement owns the session.
func (d *Deps) startAgentTerminalStream(bridge *TerminalBridge, persistentID string, wsConn *websocket.Conn, message agentmgr.Message) error {
	d.agentStreamMu.Lock()
	defer d.agentStreamMu.Unlock()
	bridge.OutputMu.Lock()
	defer bridge.OutputMu.Unlock()
	select {
	case <-bridge.ClosedCh:
		return errors.New("terminal stream closed before start")
	default:
	}
	if !d.ownsAgentTerminalStream(bridge, persistentID, wsConn) {
		return errors.New("terminal stream replaced before start")
	}
	return bridge.AgentConn.Send(message)
}

// A public session can briefly own several wire streams during reconnect.
// Saved-session deletion must close each concrete stream, including an older
// attachment whose browser handler has not finished unwinding yet.
func (d *Deps) closeAgentTerminalAttachments(session terminal.Session) error {
	var conn *agentmgr.AgentConn
	if d.AgentMgr != nil {
		conn, _ = d.AgentMgr.Get(strings.TrimSpace(session.Target))
	}
	found := false
	var closeErr error
	if d.TerminalBridges != nil {
		d.TerminalBridges.Range(func(key, value any) bool {
			bridge, ok := value.(*TerminalBridge)
			if !ok || (bridge.SessionID != session.ID && key != session.ID) {
				return true
			}
			wireID, ok := key.(string)
			if !ok {
				return true
			}
			found = true
			owner := bridge.AgentConn
			if owner == nil {
				owner = conn
			}
			bridge.CloseWithReason("persistent_session_deleted")
			if owner != nil {
				if err := SendTerminalCloseWithError(owner, wireID); err != nil && closeErr == nil {
					closeErr = err
				}
			}
			return true
		})
	}
	if !found && conn != nil {
		// Recover old pre-wire-ID plain shells recorded by earlier hubs.
		return SendTerminalCloseWithError(conn, session.ID)
	}
	return closeErr
}

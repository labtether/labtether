package terminal

import (
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/terminal"
)

// ClaimPersistentTerminalStream publishes only browser ownership. Call activate
// outside an agent lifecycle gate: persistence and closing the prior browser can
// block. Both callbacks retain the exact browser owner across replacements.
func (d *Deps) ClaimPersistentTerminalStream(session terminal.Session, conn *websocket.Conn) (activate func(), detach func()) {
	persistentID := strings.TrimSpace(session.PersistentSessionID)
	if persistentID == "" {
		return func() {}, func() {}
	}
	// Close agent writers for this saved shell before another transport owns
	// it, so they cannot append to the shared scrollback. Swap publishes
	// without waiting for an older store write; activation rechecks ownership.
	d.agentStreamMu.Lock()
	d.closePersistentAgentStreamsLocked(persistentID)
	previous, loaded := d.ActivePersistentConns.Swap(persistentID, conn)
	d.agentStreamMu.Unlock()
	activate = func() {
		d.markPersistentTerminalStreamAttached(persistentID, conn)
		if old, ok := previous.(*websocket.Conn); loaded && ok && old != conn {
			closePersistentBrowser(old)
		}
	}
	detach = func() { d.detachPersistentTerminalStream(session, conn, false) }
	return activate, detach
}

// TrackPersistentTerminalStream pairs attachment publication with the exact
// browser owner for transports without an agent connection lifecycle.
func (d *Deps) TrackPersistentTerminalStream(session terminal.Session, conn *websocket.Conn) func() {
	activate, detach := d.ClaimPersistentTerminalStream(session, conn)
	activate()
	return detach
}

// markPersistentTerminalStreamAttached is serialized with detach and only
// records an attachment that this browser still owns.
func (d *Deps) markPersistentTerminalStreamAttached(persistentID string, conn *websocket.Conn) {
	d.persistentStreamMu.Lock()
	defer d.persistentStreamMu.Unlock()
	current, present := d.ActivePersistentConns.Load(persistentID)
	if present && current == conn && d.TerminalPersistentStore != nil {
		_, _ = d.TerminalPersistentStore.MarkPersistentSessionAttached(persistentID, time.Now().UTC())
	}
}

// agentShellExitedReason is what an agent sends when the shell ended by itself.
const agentShellExitedReason = "shell exited"

// agentTmuxDetachedReason is what an agent sends when its tmux client ended
// and the tmux session is still running, or it could not tell.
const agentTmuxDetachedReason = "tmux client detached"

// persistentShellEnded reports whether the agent closed this stream because
// the saved shell itself ended.
func persistentShellEnded(bridge *TerminalBridge, agentConn *agentmgr.AgentConn) bool {
	return bridge.CloseReasonOr("") == agentShellExitedReason &&
		strings.EqualFold(strings.TrimSpace(agentConn.Meta(agentmgr.MetaTerminalReportsTmuxDetach)), "true")
}

// detachPersistentTerminalStream detaches only the exact current owner; a
// replaced browser finishing later must not detach its successor. A saved
// shell that ended has nothing left to attach to, so it moves to Ended with
// its scroll-back instead of staying listed as running.
func (d *Deps) detachPersistentTerminalStream(session terminal.Session, conn *websocket.Conn, shellEnded bool) {
	persistentID := strings.TrimSpace(session.PersistentSessionID)
	if persistentID == "" {
		return
	}
	d.persistentStreamMu.Lock()
	defer d.persistentStreamMu.Unlock()
	if !d.ActivePersistentConns.CompareAndDelete(persistentID, conn) {
		return
	}
	d.markPersistentTerminalStreamDetached(session)
	if shellEnded && d.TerminalPersistentStore != nil {
		if _, err := d.TerminalPersistentStore.MarkPersistentSessionArchived(persistentID, time.Now().UTC()); err != nil {
			log.Printf("terminal-agent: mark_ended_failed persistent=%s err=%v", persistentID, err)
		}
	}
}

func closePersistentBrowser(conn *websocket.Conn) {
	if conn == nil {
		return
	}
	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session attached elsewhere"),
		time.Now().Add(2*time.Second))
	_ = conn.Close()
}

// MarkPersistentTerminalStreamDetached handles routes without an owned browser
// registration. Do not detach a live stream when another route failed to open.
func (d *Deps) MarkPersistentTerminalStreamDetached(session terminal.Session) {
	d.persistentStreamMu.Lock()
	defer d.persistentStreamMu.Unlock()
	if _, active := d.ActivePersistentConns.Load(strings.TrimSpace(session.PersistentSessionID)); active {
		return
	}
	d.markPersistentTerminalStreamDetached(session)
}

func (d *Deps) markPersistentTerminalStreamDetached(session terminal.Session) {
	if d.TerminalPersistentStore == nil {
		return
	}
	persistentID := strings.TrimSpace(session.PersistentSessionID)
	if persistentID == "" {
		return
	}
	if d.TerminalInMemStore != nil && d.TerminalScrollbackStore != nil {
		ring := d.TerminalInMemStore.GetOrCreateScrollbackBuffer(persistentID)
		if snapshot := ring.Snapshot(); len(snapshot) > 0 {
			_ = d.TerminalScrollbackStore.UpsertScrollback(persistentID, snapshot, ring.ByteSize(), ring.Lines())
		}
	}
	_, _ = d.TerminalPersistentStore.MarkPersistentSessionDetached(persistentID, time.Now().UTC())
}

package desktop

import (
	"context"
	"errors"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// publishDesktopStream registers inside the logical-session gate and the agent
// lifecycle, so DELETE or an agent replacement cannot miss the new stream.
func (d *Deps) publishDesktopStream(ctx context.Context, bridge *DesktopBridge) bool {
	return d.publishDesktopOwner(ctx, bridge.AgentConn, func() { d.registerDesktopStream(bridge) }, bridge.Close)
}

func (d *Deps) publishWebRTCStream(ctx context.Context, bridge *WebRTCSignalingBridge) bool {
	return d.publishDesktopOwner(ctx, bridge.AgentConn, func() { d.registerWebRTCStream(bridge) }, bridge.Close)
}

func (d *Deps) publishDesktopOwner(ctx context.Context, conn *agentmgr.AgentConn, register func(), closeStream func()) bool {
	published := false
	open := shared.RunIfSessionStreamOpen(ctx, func() {
		if d.AgentMgr == nil {
			register()
			published = true
			return
		}
		published = d.AgentMgr.DispatchIfCurrent(conn, register)
	})
	if !open || !published {
		closeStream()
		return false
	}
	return true
}

// Registration pairs public ownership with wire dispatch. Replaced streams can
// unwind later, but cannot delete, stop, or receive frames for their successor.
func (d *Deps) registerDesktopStream(bridge *DesktopBridge) {
	d.streamOwnerMu.Lock()
	defer d.streamOwnerMu.Unlock()
	if !d.currentDesktopAgent(bridge.AgentConn) {
		bridge.Close()
		return
	}
	d.DesktopBridges.Store(bridge.WireSessionID, bridge)
	previous, _ := d.desktopOwners.Swap(bridge.SessionID, bridge)
	closeReplacedDesktopStream(previous)
}

func (d *Deps) loadDesktopOwner(sessionID string) (any, bool) {
	if owner, ok := d.desktopOwners.Load(sessionID); ok {
		return owner, true
	}
	// Keep direct in-process bridge callers compatible. Live streams always have
	// a wire ID, and can never be addressed through this legacy public-ID path.
	if d.DesktopBridges != nil {
		if raw, ok := d.DesktopBridges.Load(sessionID); ok {
			if bridge, ok := raw.(*DesktopBridge); ok && bridge.WireSessionID == "" {
				return bridge, true
			}
		}
	}
	return nil, false
}

func (b *DesktopBridge) start(message agentmgr.Message) error {
	b.lifecycleMu.Lock()
	defer b.lifecycleMu.Unlock()
	select {
	case <-b.ClosedCh:
		return errors.New("desktop stream closed before start")
	default:
	}
	if err := b.AgentConn.Send(message); err != nil {
		return err
	}
	b.started = true
	return nil
}

func (d *Deps) registerWebRTCStream(bridge *WebRTCSignalingBridge) {
	d.streamOwnerMu.Lock()
	defer d.streamOwnerMu.Unlock()
	if !d.currentDesktopAgent(bridge.AgentConn) {
		bridge.Close()
		return
	}
	d.WebRTCBridges.Store(bridge.WireSessionID, bridge)
	previous, _ := d.desktopOwners.Swap(bridge.SessionID, bridge)
	closeReplacedDesktopStream(previous)
}

func (d *Deps) finalizeWebRTCStream(bridge *WebRTCSignalingBridge) {
	bridge.Close()
	d.WebRTCBridges.CompareAndDelete(bridge.WireSessionID, bridge)
	d.desktopOwners.CompareAndDelete(bridge.SessionID, bridge)
	bridge.stop()
}

func (b *WebRTCSignalingBridge) start(message agentmgr.Message) error {
	b.lifecycleMu.Lock()
	defer b.lifecycleMu.Unlock()
	select {
	case <-b.ClosedCh:
		return errors.New("webrtc stream closed before start")
	default:
	}
	if err := b.AgentConn.Send(message); err != nil {
		return err
	}
	b.started = true
	return nil
}

func (b *WebRTCSignalingBridge) stop() {
	b.lifecycleMu.Lock()
	defer b.lifecycleMu.Unlock()
	if b.started && !b.stopSent {
		b.stopSent = true
		SendWebRTCStop(b.AgentConn, b.WireSessionID)
	}
}

// Session deletion also closes any replaced stream still unwinding.
func (d *Deps) closeDesktopSessionStreams(sessionID string) {
	d.audioStreams.Range(func(_ any, value any) bool {
		if audio, ok := value.(*DesktopBridge); ok && audio.SessionID == sessionID {
			d.finalizeDesktopAudioStream(audio)
		}
		return true
	})
	if d.DesktopBridges != nil {
		d.DesktopBridges.Range(func(key, value any) bool {
			if bridge, ok := value.(*DesktopBridge); ok && (bridge.SessionID == sessionID || key == sessionID) {
				d.FinalizeAgentDesktopSession(sessionID, bridge, bridge.AgentConn, false, nil)
			}
			return true
		})
	}
	if d.WebRTCBridges != nil {
		d.WebRTCBridges.Range(func(key, value any) bool {
			if bridge, ok := value.(*WebRTCSignalingBridge); ok && (bridge.SessionID == sessionID || key == sessionID) {
				d.WebRTCBridges.CompareAndDelete(key, bridge)
				d.finalizeWebRTCStream(bridge)
			}
			return true
		})
	}
}

// A reconnect can replace AgentManager's asset entry before the old socket's
// deferred cleanup runs. Close only streams bound to that exact old socket.
func (d *Deps) CloseDesktopStreamsForConnection(conn *agentmgr.AgentConn) {
	if conn == nil {
		return
	}
	if d.DesktopBridges != nil {
		d.DesktopBridges.Range(func(_ any, value any) bool {
			if bridge, ok := value.(*DesktopBridge); ok && bridge.AgentConn == conn {
				bridge.Close()
			}
			return true
		})
	}
	if d.WebRTCBridges != nil {
		d.WebRTCBridges.Range(func(_ any, value any) bool {
			if bridge, ok := value.(*WebRTCSignalingBridge); ok && bridge.AgentConn == conn {
				bridge.Close()
			}
			return true
		})
	}
}

func closeReplacedDesktopStream(previous any) {
	switch old := previous.(type) {
	case *DesktopBridge:
		old.SetReason("session attached elsewhere")
		old.Close()
	case *WebRTCSignalingBridge:
		old.Close()
	}
}

func (d *Deps) currentDesktopAgent(conn *agentmgr.AgentConn) bool {
	if d.AgentMgr == nil {
		return true
	}
	if conn == nil {
		return false
	}
	current, ok := d.AgentMgr.Get(conn.AssetID)
	return ok && current == conn
}

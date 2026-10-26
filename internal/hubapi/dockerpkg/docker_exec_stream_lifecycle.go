package dockerpkg

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
)

// publishDockerExecStream registers the stream inside the logical-session
// gate and the agent lifecycle, so DELETE or an agent replacement cannot miss
// it. A browser that captured a replaced agent cannot claim the session.
// Persistent activation can block, so it runs after the agent gate.
func (d *Deps) publishDockerExecStream(ctx context.Context, bridge *DockerExecBridge, session terminal.Session, wsConn *websocket.Conn) (func(), bool) {
	published := false
	activate, detach := func() {}, func() {}
	open := shared.RunIfSessionStreamOpen(ctx, func() {
		published = d.AgentMgr.DispatchIfCurrent(bridge.AgentConn, func() {
			d.registerDockerExecStream(bridge)
			if d.ClaimPersistentTerminalStream != nil {
				activate, detach = d.ClaimPersistentTerminalStream(session, wsConn)
			}
		})
		if published {
			activate()
		}
	})
	if !open || !published {
		bridge.Close("container host agent disconnected")
		return func() {}, false
	}
	return detach, true
}

// Wire dispatch and public ownership are separate: an old handler may unwind
// after its successor starts, but cannot delete or stop that successor.
func (d *Deps) registerDockerExecStream(bridge *DockerExecBridge) {
	d.dockerExecOwnerMu.Lock()
	if d.AgentMgr != nil {
		current, connected := d.AgentMgr.Get(bridge.ExpectedAgentID)
		if !connected || current != bridge.AgentConn {
			d.dockerExecOwnerMu.Unlock()
			bridge.Close("container host agent disconnected")
			return
		}
	}
	d.DockerExecBridges.Store(bridge.WireSessionID, bridge)
	previous, _ := d.dockerExecOwners.Swap(bridge.SessionID, bridge)
	d.dockerExecOwnerMu.Unlock()
	if old, ok := previous.(*DockerExecBridge); ok {
		old.Close("session attached elsewhere")
	}
}

func (d *Deps) finalizeDockerExecStream(bridge *DockerExecBridge) {
	bridge.Close("")
	owned := d.DockerExecBridges.CompareAndDelete(bridge.WireSessionID, bridge)
	d.dockerExecOwners.CompareAndDelete(bridge.SessionID, bridge)
	if owned {
		bridge.stop()
	}
}

func (d *Deps) startDockerExecStream(bridge *DockerExecBridge, message agentmgr.Message) error {
	d.dockerExecOwnerMu.Lock()
	defer d.dockerExecOwnerMu.Unlock()
	if owner, _ := d.dockerExecOwners.Load(bridge.SessionID); owner != bridge {
		return errors.New("container terminal replaced before start")
	}
	return bridge.start(message)
}

// A replacement can close a bridge while its browser is still in startup.
// Serialize the start with closure so an evicted stream cannot start afterward.
func (b *DockerExecBridge) start(message agentmgr.Message) error {
	b.lifecycleMu.Lock()
	defer b.lifecycleMu.Unlock()
	select {
	case <-b.ClosedCh:
		return errors.New("container terminal closed before start")
	default:
	}
	if err := b.AgentConn.Send(message); err != nil {
		return err
	}
	b.started = true
	return nil
}

func (b *DockerExecBridge) stop() {
	b.lifecycleMu.Lock()
	defer b.lifecycleMu.Unlock()
	if !b.started {
		return
	}
	b.started = false
	data, _ := json.Marshal(agentmgr.DockerExecCloseData{SessionID: b.WireSessionID})
	_ = b.AgentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgDockerExecClose,
		ID:   b.WireSessionID,
		Data: data,
	})
}

// An old socket's deferred disconnect must leave the replacement socket's
// streams intact, even though both sockets share the same asset ID.
func (d *Deps) CloseDockerExecBridgesForConnection(conn *agentmgr.AgentConn) {
	if conn == nil || d.DockerExecBridges == nil {
		return
	}
	d.DockerExecBridges.Range(func(_ any, value any) bool {
		if bridge, ok := value.(*DockerExecBridge); ok && bridge.AgentConn == conn {
			bridge.Close("container host agent disconnected")
		}
		return true
	})
}

// ProcessAgentDockerExecStarted handles the docker.exec.started agent message.
func (d *Deps) ProcessAgentDockerExecStarted(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.DockerExecStartedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	if bridge, ok := d.DockerExecBridges.Load(data.SessionID); ok {
		if b, ok := bridge.(*DockerExecBridge); ok {
			if !b.acceptsAgentMessage(conn, data.SessionID) {
				return
			}
			b.TrySendOutput(nil)
		}
	}
}

// ProcessAgentDockerExecData handles the docker.exec.data agent message.
func (d *Deps) ProcessAgentDockerExecData(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var payload agentmgr.DockerExecDataPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil || len(decoded) == 0 {
		return
	}
	if bridge, ok := d.DockerExecBridges.Load(payload.SessionID); ok {
		if b, ok := bridge.(*DockerExecBridge); ok {
			if !b.acceptsAgentMessage(conn, payload.SessionID) {
				return
			}
			b.TrySendOutput(decoded)
		}
	}
}

// ProcessAgentDockerExecClosed handles the docker.exec.closed agent message.
func (d *Deps) ProcessAgentDockerExecClosed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.DockerExecCloseData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	if bridge, ok := d.DockerExecBridges.Load(data.SessionID); ok {
		if b, ok := bridge.(*DockerExecBridge); ok {
			if !b.acceptsAgentMessage(conn, data.SessionID) {
				return
			}
			b.Close(data.Reason)
		}
	}
}

// CloseDockerExecBridgesForAsset closes all docker exec bridges for the given asset.
func (d *Deps) CloseDockerExecBridgesForAsset(assetID string) {
	trimmedAssetID := strings.TrimSpace(assetID)
	if trimmedAssetID == "" {
		return
	}
	d.DockerExecBridges.Range(func(_ any, value any) bool {
		bridge, ok := value.(*DockerExecBridge)
		if !ok {
			return true
		}
		if bridge.MatchesAgent(trimmedAssetID) {
			bridge.Close("container host agent disconnected")
		}
		return true
	})
}

// acceptsAgentMessage binds replies to the stream's wire ID and socket owner.
func (b *DockerExecBridge) acceptsAgentMessage(conn *agentmgr.AgentConn, wireID string) bool {
	return b != nil && b.MatchesConnection(conn) && (b.WireSessionID == "" || b.WireSessionID == wireID)
}

// Browser event consumers use the logical Hub session ID. The wire ID belongs
// only to this socket instance and must not replace that public correlation ID.
func (d *Deps) broadcastDockerExecMessage(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	if d.Broadcast == nil || d.DockerExecBridges == nil {
		return
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(msg.Data, &payload) != nil {
		return
	}
	var wireID string
	if json.Unmarshal(payload["session_id"], &wireID) != nil {
		return
	}
	raw, ok := d.DockerExecBridges.Load(wireID)
	bridge, valid := raw.(*DockerExecBridge)
	if !ok || !valid || !bridge.acceptsAgentMessage(conn, wireID) {
		return
	}
	// Serialize public event publication with logical-owner replacement.
	d.dockerExecOwnerMu.Lock()
	defer d.dockerExecOwnerMu.Unlock()
	if bridge.SessionID != "" {
		// Superseded streams can still finish their private teardown. They cannot
		// send public lifecycle events that appear to belong to the replacement.
		if current, exists := d.dockerExecOwners.Load(bridge.SessionID); !exists || current != bridge {
			return
		}
		payload["session_id"], _ = json.Marshal(bridge.SessionID)
	}
	data, err := json.Marshal(payload)
	if err == nil {
		d.Broadcast(msg.Type, data)
	}
}

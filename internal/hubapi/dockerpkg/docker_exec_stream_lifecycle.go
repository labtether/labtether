package dockerpkg

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
)

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
			if !b.MatchesConnection(conn) {
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
			if !b.MatchesConnection(conn) {
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
			if !b.MatchesConnection(conn) {
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

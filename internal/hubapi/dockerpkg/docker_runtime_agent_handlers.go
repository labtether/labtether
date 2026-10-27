package dockerpkg

import (
	"encoding/json"

	"github.com/labtether/labtether/internal/agentmgr"
)

// ProcessAgentDockerDiscovery handles docker.discovery messages from agents.
func (d *Deps) ProcessAgentDockerDiscovery(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	if d.DockerCoordinator != nil {
		d.DockerCoordinator.HandleDiscovery(conn.AssetID, msg)
	}
	if d.TriggerDockerCollectorRunForDiscovery != nil {
		d.TriggerDockerCollectorRunForDiscovery()
	}
}

// ProcessAgentDockerDiscoveryDelta handles docker.discovery.delta messages from agents.
func (d *Deps) ProcessAgentDockerDiscoveryDelta(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	if d.DockerCoordinator != nil {
		d.DockerCoordinator.HandleDiscoveryDelta(conn.AssetID, msg)
	}
	if d.TriggerDockerCollectorRunForDiscovery != nil {
		d.TriggerDockerCollectorRunForDiscovery()
	}
}

// ProcessAgentDockerStats handles docker.stats messages from agents.
func (d *Deps) ProcessAgentDockerStats(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	if d.DockerCoordinator != nil {
		d.DockerCoordinator.HandleStats(conn.AssetID, msg)
	}
}

// ProcessAgentDockerEvents handles docker.events messages from agents.
func (d *Deps) ProcessAgentDockerEvents(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	if d.DockerCoordinator != nil {
		d.DockerCoordinator.HandleEvent(conn.AssetID, msg)
		if d.Broadcast != nil {
			if event, ok := attributedDockerEvent(conn.AssetID, msg.Data); ok {
				d.Broadcast("docker.event", event)
			}
		}
	}
}

// attributedDockerEvent re-encodes an agent's Docker event as the known
// fields only, with host_id set to the sending agent, so an agent cannot pass
// arbitrary JSON or speak for another host.
func attributedDockerEvent(assetID string, raw json.RawMessage) (json.RawMessage, bool) {
	var event agentmgr.DockerEventData
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, false
	}
	event.HostID = assetID
	encoded, err := json.Marshal(event)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

// ProcessAgentDockerActionResult handles docker.action.result messages from agents.
func (d *Deps) ProcessAgentDockerActionResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	if d.DockerCoordinator != nil {
		d.DockerCoordinator.HandleActionResult(conn.AssetID, msg)
	}
}

// ProcessAgentDockerExecStartedMessage handles docker.exec.started messages from agents.
func (d *Deps) ProcessAgentDockerExecStartedMessage(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	d.ProcessAgentDockerExecStarted(conn, msg)
	d.broadcastDockerExecMessage(conn, msg)
}

// ProcessAgentDockerExecDataMessage handles docker.exec.data messages from agents.
func (d *Deps) ProcessAgentDockerExecDataMessage(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	d.ProcessAgentDockerExecData(conn, msg)
	d.broadcastDockerExecMessage(conn, msg)
}

// ProcessAgentDockerExecClosedMessage handles docker.exec.closed messages from agents.
func (d *Deps) ProcessAgentDockerExecClosedMessage(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	d.ProcessAgentDockerExecClosed(conn, msg)
	d.broadcastDockerExecMessage(conn, msg)
}

// ProcessAgentDockerLogsStreamMessage drops docker.logs.stream messages. The
// Hub never starts these streams (container logs use request/response), so
// any that arrive are unsolicited and are not passed to browsers or webhooks.
func (d *Deps) ProcessAgentDockerLogsStreamMessage(_ *agentmgr.AgentConn, _ agentmgr.Message) {}

// ProcessAgentDockerComposeResult handles docker.compose.result messages from agents.
func (d *Deps) ProcessAgentDockerComposeResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	if d.DockerCoordinator != nil {
		d.DockerCoordinator.HandleComposeResult(conn.AssetID, msg)
	}
}

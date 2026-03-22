package dockerpkg

import "github.com/labtether/labtether/internal/agentmgr"

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
			d.Broadcast("docker.event", msg.Data)
		}
	}
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
	if d.Broadcast != nil {
		d.Broadcast(msg.Type, msg.Data)
	}
}

// ProcessAgentDockerExecDataMessage handles docker.exec.data messages from agents.
func (d *Deps) ProcessAgentDockerExecDataMessage(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	d.ProcessAgentDockerExecData(conn, msg)
	if d.Broadcast != nil {
		d.Broadcast(msg.Type, msg.Data)
	}
}

// ProcessAgentDockerExecClosedMessage handles docker.exec.closed messages from agents.
func (d *Deps) ProcessAgentDockerExecClosedMessage(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	d.ProcessAgentDockerExecClosed(conn, msg)
	if d.Broadcast != nil {
		d.Broadcast(msg.Type, msg.Data)
	}
}

// ProcessAgentDockerLogsStreamMessage handles docker.logs.stream messages from agents.
func (d *Deps) ProcessAgentDockerLogsStreamMessage(_ *agentmgr.AgentConn, msg agentmgr.Message) {
	// Forward log stream to the browser via broadcaster.
	if d.Broadcast != nil {
		d.Broadcast(msg.Type, msg.Data)
	}
}

// ProcessAgentDockerComposeResult handles docker.compose.result messages from agents.
func (d *Deps) ProcessAgentDockerComposeResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	if d.DockerCoordinator != nil {
		d.DockerCoordinator.HandleComposeResult(conn.AssetID, msg)
	}
}

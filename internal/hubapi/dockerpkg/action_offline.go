package dockerpkg

import (
	"strings"

	"github.com/labtether/labtether/internal/connectorsdk"
)

// offlineTargetError replaces a failed target lookup with a plain error when
// the Docker host's agent is offline, or connected but has not sent its first
// Docker report since it connected. The Hub forgets a host's containers when
// its agent disconnects, so a lookup then fails with an internal reason that
// means nothing to the person who pressed Stop.
func (d *Deps) offlineTargetError(targetID string, lookupErr error) error {
	if d.AssetStore == nil || d.AgentMgr == nil {
		return lookupErr
	}
	target, ok, err := d.AssetStore.GetAsset(strings.TrimSpace(targetID))
	if err != nil || !ok {
		return lookupErr
	}
	agentID := strings.TrimSpace(target.Metadata["agent_id"])
	if strings.EqualFold(strings.TrimSpace(target.Source), "agent") {
		agentID = target.ID
	}
	if agentID == "" {
		return lookupErr
	}
	unavailable := &connectorsdk.TargetUnavailableError{Device: agentID, What: "containers"}
	if d.AgentMgr.IsConnected(agentID) {
		// Connected, but the first Docker report after a reconnect has not
		// come in yet, so the Hub does not know the host's containers.
		if d.DockerCoordinator == nil {
			return lookupErr
		}
		if _, reported := d.DockerCoordinator.GetHost(agentID); reported {
			return lookupErr
		}
		unavailable.NotReported = true
	}
	if agent, ok, err := d.AssetStore.GetAsset(agentID); err == nil && ok {
		if name := strings.TrimSpace(agent.Name); name != "" {
			unavailable.Device = name
		}
		unavailable.LastSeen = agent.LastSeenAt
	}
	return unavailable
}

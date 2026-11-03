package dockerpkg

import (
	"strings"

	"github.com/labtether/labtether/internal/connectorsdk"
)

// offlineTargetError replaces a failed target lookup with a plain "offline"
// error when the Docker host's agent is not connected. The Hub forgets a
// host's containers when its agent disconnects, so a lookup then fails with
// an internal reason that means nothing to the person who pressed Stop.
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
	if agentID == "" || d.AgentMgr.IsConnected(agentID) {
		return lookupErr
	}
	offline := &connectorsdk.TargetOfflineError{Device: agentID}
	if agent, ok, err := d.AssetStore.GetAsset(agentID); err == nil && ok {
		if name := strings.TrimSpace(agent.Name); name != "" {
			offline.Device = name
		}
		offline.LastSeen = agent.LastSeenAt
	}
	return offline
}

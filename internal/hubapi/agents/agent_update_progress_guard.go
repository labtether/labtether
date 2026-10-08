package agents

import (
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

type activeAgentUpdate struct {
	assetID   string
	expiresAt time.Time
}

// trackAgentUpdate binds progress to a Hub-issued request. Both operator and
// automatic updates use this registry; the shared pending-command map only
// covers operator requests and also contains non-update commands.
func (d *Deps) trackAgentUpdate(jobID, assetID string, ttl time.Duration) func() {
	now := time.Now()
	d.activeAgentUpdates.Range(func(key, value any) bool {
		entry := value.(*activeAgentUpdate)
		if !entry.expiresAt.After(now) {
			d.activeAgentUpdates.CompareAndDelete(key, entry)
		}
		return true
	})
	entry := &activeAgentUpdate{assetID: assetID, expiresAt: now.Add(ttl)}
	d.activeAgentUpdates.Store(jobID, entry)
	return func() { d.activeAgentUpdates.CompareAndDelete(jobID, entry) }
}

func (d *Deps) acceptsAgentUpdate(conn *agentmgr.AgentConn, jobID string) bool {
	if conn == nil || jobID == "" {
		return false
	}
	raw, ok := d.activeAgentUpdates.Load(jobID)
	if !ok {
		return false
	}
	entry := raw.(*activeAgentUpdate)
	if !entry.expiresAt.After(time.Now()) {
		d.activeAgentUpdates.CompareAndDelete(jobID, entry)
		return false
	}
	return entry.assetID == conn.AssetID
}

func (d *Deps) finishAgentUpdate(conn *agentmgr.AgentConn, jobID string) {
	if conn == nil || jobID == "" {
		return
	}
	raw, ok := d.activeAgentUpdates.Load(jobID)
	if ok && raw.(*activeAgentUpdate).assetID == conn.AssetID {
		d.activeAgentUpdates.CompareAndDelete(jobID, raw)
	}
}

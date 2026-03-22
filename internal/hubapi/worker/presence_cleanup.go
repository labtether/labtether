package worker

import (
	"context"
	"log"
	"time"
)

const PresenceCleanupInterval = 60 * time.Second
const PresenceStaleThreshold = 60 * time.Second

// RunPresenceCleanup periodically removes stale presence records where
// the agent hasn't sent a heartbeat within the threshold. This handles
// cases where the WebSocket disconnect wasn't cleanly detected.
func (d *Deps) RunPresenceCleanup(ctx context.Context) {
	ticker := time.NewTicker(PresenceCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.cleanupStalePresence()
		}
	}
}

func (d *Deps) cleanupStalePresence() {
	if d.PresenceStore == nil {
		return
	}

	cutoff := time.Now().UTC().Add(-PresenceStaleThreshold)
	stale, err := d.PresenceStore.GetStalePresence(cutoff)
	if err != nil {
		log.Printf("presence cleanup: failed to query stale records: %v", err)
		return
	}

	for _, p := range stale {
		// Skip if the agent is still registered in-memory (heartbeat timestamps
		// may lag behind the in-memory connection state).
		if d.AgentMgr != nil && d.AgentMgr.IsConnected(p.AssetID) {
			continue
		}

		deleted, err := d.PresenceStore.DeletePresenceForSession(p.AssetID, p.SessionID)
		if err != nil {
			log.Printf("presence cleanup: failed to delete %s: %v", p.AssetID, err)
			continue
		}
		if !deleted {
			continue
		}
		_ = d.PresenceStore.UpdateAssetTransportType(p.AssetID, "offline")
		// Note: we intentionally don't call Unregister here to avoid a TOCTOU race
		// where an agent reconnects between our IsConnected check and Unregister.
		// The DB presence record is the source of truth; in-memory cleanup happens
		// in handleAgentWebSocket's defer on disconnect.
		log.Printf("presence cleanup: removed stale agent %s (last heartbeat: %s)",
			p.AssetID, p.LastHeartbeatAt.Format(time.RFC3339))
	}
}

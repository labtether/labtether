package resources

// asset_helpers.go — pure, stateless helpers for asset classification used
// by both the heartbeat/delete handlers and the infra cascade logic.
// These functions have no apiServer or Deps dependency.

import (
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
)

// isConnectedAgentAsset reports whether the asset is currently connected
// via a live agent WebSocket and is sourced from the "agent" source.
// Returns false when mgr is nil.
func isConnectedAgentAsset(assetEntry assets.Asset, mgr *agentmgr.AgentManager) bool {
	if mgr == nil {
		return false
	}
	if !mgr.IsConnected(strings.TrimSpace(assetEntry.ID)) {
		return false
	}

	switch NormalizeSource(assetEntry.Source) {
	case "agent":
		return true
	default:
		return false
	}
}

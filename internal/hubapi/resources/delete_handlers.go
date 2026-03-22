package resources

// delete_handlers.go — DELETE /assets/{id} cascade orchestration extracted
// from cmd/labtether/assets_heartbeat_handlers.go.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

// HandleDeleteAsset handles DELETE /assets/{id}. It decommissions an asset
// with best-effort SSH key cleanup, token revocation, coordinator removal,
// attached child cascade, and final store delete.
func (d *Deps) HandleDeleteAsset(w http.ResponseWriter, assetEntry assets.Asset) {
	assetID := strings.TrimSpace(assetEntry.ID)

	if isConnectedAgentAsset(assetEntry, d.AgentMgr) {
		if d.SendSSHKeyRemoveToAsset != nil {
			d.SendSSHKeyRemoveToAsset(assetID)
		}
		servicehttp.WriteError(w, http.StatusConflict, "connected agent assets must be stopped or uninstalled before deletion")
		return
	}

	// Best-effort: ask agent to remove hub SSH key before disconnecting.
	if d.SendSSHKeyRemoveToAsset != nil {
		if d.AgentMgr != nil {
			if _, ok := d.AgentMgr.Get(assetID); ok {
				d.SendSSHKeyRemoveToAsset(assetID)
			}
		}
	}

	// Revoke agent tokens (agent_tokens has no FK cascade to assets).
	if d.EnrollmentStore != nil {
		_ = d.EnrollmentStore.RevokeAgentTokensByAsset(assetID)
	}
	if err := d.deleteAutoDockerCollectorsForAsset(assetID); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete attached docker collectors")
		return
	}

	// Disconnect agent WebSocket connection.
	if d.AgentMgr != nil {
		d.AgentMgr.Unregister(assetID)
	}
	if d.RemoveDockerHost != nil {
		d.RemoveDockerHost(assetID)
	}
	if d.RemoveWebServiceHost != nil {
		d.RemoveWebServiceHost(assetID)
	}

	// Remove attached infrastructure children for non-docker host sources.
	if err := d.deleteAttachedInfraAssets(assetEntry); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete attached infrastructure assets")
		return
	}

	// Always attempt docker-child cleanup for the deleted asset ID.
	// This covers non-agent parent assets (for example connector-discovered hosts)
	// that still own docker children via metadata.agent_id links.
	if err := d.deleteAttachedDockerAssets(assetID); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete attached docker assets")
		return
	}

	// Delete asset — FK cascades handle child rows (heartbeats, metrics, etc.).
	if err := d.AssetStore.DeleteAsset(assetID); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "asset not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete asset")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"deleted":  true,
		"asset_id": assetID,
	})
}

func (d *Deps) deleteAttachedDockerAssets(agentAssetID string) error {
	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		return err
	}

	for _, candidate := range assetList {
		if !d.dockerAssetAttachedToAgent(candidate, agentAssetID) {
			continue
		}
		if err := d.AssetStore.DeleteAsset(candidate.ID); err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
	}
	return nil
}

func (d *Deps) deleteAttachedInfraAssets(parent assets.Asset) error {
	if !IsInfraHostAsset(parent) {
		return nil
	}

	parentSource := NormalizeSource(parent.Source)
	if parentSource == "docker" {
		// Docker attachments rely on metadata.agent_id and legacy ID patterns
		// handled by deleteAttachedDockerAssets.
		return nil
	}

	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		return err
	}

	for _, candidate := range assetList {
		if !InfraChildAttachedToParent(parent, candidate) {
			continue
		}
		if err := d.AssetStore.DeleteAsset(candidate.ID); err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
	}
	return nil
}

func (d *Deps) deleteAutoDockerCollectorsForAsset(assetID string) error {
	trimmedAssetID := strings.TrimSpace(assetID)
	if trimmedAssetID == "" || d.HubCollectorStore == nil {
		return nil
	}

	collectors, err := d.HubCollectorStore.ListHubCollectors(500, false)
	if err != nil {
		return err
	}

	for _, collector := range collectors {
		if !strings.EqualFold(strings.TrimSpace(collector.CollectorType), hubcollector.CollectorTypeDocker) {
			continue
		}

		var targetAssetID string
		if d.CollectorConfigString != nil {
			targetAssetID = strings.TrimSpace(d.CollectorConfigString(collector.Config, "agent_asset_id"))
		}
		if targetAssetID != trimmedAssetID {
			continue
		}

		if err := d.HubCollectorStore.DeleteHubCollector(collector.ID); err != nil && !errors.Is(err, hubcollector.ErrCollectorNotFound) {
			return err
		}

		clusterAssetID := strings.TrimSpace(collector.AssetID)
		if clusterAssetID == "" || strings.EqualFold(clusterAssetID, trimmedAssetID) {
			continue
		}
		if err := d.AssetStore.DeleteAsset(clusterAssetID); err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
	}

	return nil
}

// dockerAssetAttachedToAgent reports whether candidate is a Docker asset
// owned by agentAssetID, checked via metadata.agent_id or legacy ID patterns.
func (d *Deps) dockerAssetAttachedToAgent(assetEntry assets.Asset, agentAssetID string) bool {
	if !strings.EqualFold(strings.TrimSpace(assetEntry.Source), "docker") {
		return false
	}

	agentAssetID = strings.TrimSpace(agentAssetID)
	if agentAssetID == "" {
		return false
	}

	if strings.TrimSpace(assetEntry.Metadata["agent_id"]) == agentAssetID {
		return true
	}

	var normalizedAgentID string
	if d.NormalizeAssetKey != nil {
		normalizedAgentID = d.NormalizeAssetKey(agentAssetID)
	}
	if normalizedAgentID == "" {
		return false
	}

	var autoCollectorAssetID string
	if d.AutoDockerCollectorAssetID != nil {
		autoCollectorAssetID = d.AutoDockerCollectorAssetID(agentAssetID)
	}

	assetID := strings.ToLower(strings.TrimSpace(assetEntry.ID))
	if autoCollectorAssetID != "" && assetID == strings.ToLower(autoCollectorAssetID) {
		return true
	}
	if assetID == "docker-host-"+normalizedAgentID {
		return true
	}
	if strings.HasPrefix(assetID, "docker-ct-"+normalizedAgentID+"-") {
		return true
	}
	if strings.HasPrefix(assetID, "docker-stack-"+normalizedAgentID+"-") {
		return true
	}
	return false
}

package resources

// delete_handlers.go — DELETE /assets/{id} cascade orchestration extracted
// from cmd/labtether/assets_heartbeat_handlers.go.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

var ErrConnectedAgentDeletion = errors.New("connected agent assets must be stopped or uninstalled before deletion")

// HandleDeleteAsset handles DELETE /assets/{id}. It decommissions an asset
// with best-effort SSH key cleanup, token revocation, coordinator removal,
// attached child cascade, and final store delete.
func (d *Deps) HandleDeleteAsset(w http.ResponseWriter, r *http.Request, assetEntry assets.Asset) {
	if err := d.DeleteAsset(context.Background(), assetEntry); err != nil {
		switch {
		case errors.Is(err, ErrConnectedAgentDeletion):
			servicehttp.WriteError(w, http.StatusConflict, ErrConnectedAgentDeletion.Error())
		case errors.Is(err, persistence.ErrNotFound):
			servicehttp.WriteError(w, http.StatusNotFound, "device not found")
		default:
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete asset")
		}
		return
	}
	d.AuditAssetDeleted(r, assetEntry)
	assetID := strings.TrimSpace(assetEntry.ID)
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "asset_id": assetID})
}

// AuditAssetDeleted records a removed device with its name, after the
// removal succeeded (a refused removal keeps only its request row).
func (d *Deps) AuditAssetDeleted(r *http.Request, assetEntry assets.Asset) {
	if d.AppendAuditEventBestEffort == nil {
		return
	}
	event := audit.NewEvent("api.asset.deleted")
	if d.PrincipalActorID != nil {
		event.ActorID = d.PrincipalActorID(r.Context())
	}
	event.Target = strings.TrimSpace(assetEntry.ID)
	event.TargetName = strings.TrimSpace(assetEntry.Name)
	event.Details = map[string]any{"asset_id": event.Target}
	d.AppendAuditEventBestEffort(event, "api warning: failed to append device removal audit event")
}

// assetName is a device's name for an audit row, or "" when unknown.
func (d *Deps) assetName(assetID string) string {
	if d == nil || d.AssetStore == nil || strings.TrimSpace(assetID) == "" {
		return ""
	}
	assetEntry, ok, err := d.AssetStore.GetAsset(strings.TrimSpace(assetID))
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(assetEntry.Name)
}

// DeleteAsset applies the same decommission policy for both v1 and v2. Agent
// assets are removed through the persistence transaction that also revokes all
// active or prepared credentials under the per-asset identity lock.
func (d *Deps) DeleteAsset(ctx context.Context, assetEntry assets.Asset) error {
	assetID := strings.TrimSpace(assetEntry.ID)

	// A refused delete must change nothing, so this check comes before any
	// message to the agent.
	if isConnectedAgentAsset(assetEntry, d.AgentMgr) {
		return ErrConnectedAgentDeletion
	}

	// Best-effort: ask agent to remove hub SSH key before disconnecting.
	if d.SendSSHKeyRemoveToAsset != nil {
		if d.AgentMgr != nil {
			if _, ok := d.AgentMgr.Get(assetID); ok {
				d.SendSSHKeyRemoveToAsset(assetID)
			}
		}
	}

	if err := d.deleteAutoDockerCollectorsForAsset(assetID); err != nil {
		return fmt.Errorf("delete attached docker collectors: %w", err)
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
		return fmt.Errorf("delete attached infrastructure assets: %w", err)
	}
	// A connection's own device takes every device it found with it.
	ownedConnections := d.collectorsOwnedByAsset(assetID)
	if err := d.deleteConnectionDevices(ownedConnections, assetID); err != nil {
		return fmt.Errorf("delete the connection's devices: %w", err)
	}

	// Always attempt docker-child cleanup for the deleted asset ID.
	// This covers non-agent parent assets (for example connector-discovered hosts)
	// that still own docker children via metadata.agent_id links.
	if err := d.deleteAttachedDockerAssets(assetID); err != nil {
		return fmt.Errorf("delete attached docker assets: %w", err)
	}

	if d.EnrollmentTransactions != nil {
		if err := d.EnrollmentTransactions.DecommissionAgentAsset(ctx, assetID); err != nil {
			return err
		}
	} else if NormalizeSource(assetEntry.Source) == "agent" {
		return persistence.ErrAgentEnrollmentTransactionsUnavailable
	} else if err := d.AssetStore.DeleteAsset(assetID); err != nil {
		return err
	}
	// Close any connection that registered after the initial connected check but
	// before the atomic revocation/delete committed.
	if d.AgentMgr != nil {
		d.AgentMgr.Unregister(assetID)
	}
	// Only after the device (and with it the connection) is gone.
	d.deleteUnusedConnectionLogins(ownedConnections)
	// Only after the device is gone: a refused or failed delete keeps its
	// saved terminal sessions.
	if d.RemoveSavedTerminalSessions != nil {
		d.RemoveSavedTerminalSessions(ctx, assetID)
	}
	return nil
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
	collectors, err := d.autoDockerCollectorsForAsset(trimmedAssetID)
	if err != nil {
		return err
	}
	for _, collector := range collectors {
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

// autoDockerCollectorsForAsset returns the Docker connections the Hub made by
// itself for the agent device assetID.
func (d *Deps) autoDockerCollectorsForAsset(assetID string) ([]hubcollector.Collector, error) {
	trimmedAssetID := strings.TrimSpace(assetID)
	if trimmedAssetID == "" || d.HubCollectorStore == nil {
		return nil, nil
	}

	collectors, err := d.HubCollectorStore.ListHubCollectors(500, false)
	if err != nil {
		return nil, err
	}

	matched := make([]hubcollector.Collector, 0)
	for _, collector := range collectors {
		if !strings.EqualFold(strings.TrimSpace(collector.CollectorType), hubcollector.CollectorTypeDocker) {
			continue
		}

		var targetAssetID string
		if d.CollectorConfigString != nil {
			targetAssetID = strings.TrimSpace(d.CollectorConfigString(collector.Config, "agent_asset_id"))
		}
		if targetAssetID == trimmedAssetID {
			matched = append(matched, collector)
		}
	}
	return matched, nil
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

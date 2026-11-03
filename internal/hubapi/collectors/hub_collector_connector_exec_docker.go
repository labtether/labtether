package collectors

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/modelmap"
	"github.com/labtether/labtether/internal/persistence"
)

func (d *Deps) ExecuteDockerCollector(ctx context.Context, collector hubcollector.Collector) {
	if d.ConnectorRegistry == nil {
		d.UpdateCollectorStatus(collector.ID, "error", "connector registry unavailable")
		return
	}

	connector, ok := d.ConnectorRegistry.Get(hubcollector.CollectorTypeDocker)
	if !ok {
		d.UpdateCollectorStatus(collector.ID, "error", "docker connector not found")
		return
	}

	collectorCtx, collectorCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer collectorCancel()

	discovered, err := connector.Discover(collectorCtx)
	if err != nil {
		d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("docker discovery failed: %v", err))
		return
	}

	ingested := 0
	snapshotAssets := make([]connectorsdk.Asset, 0, len(discovered)+1)
	for _, discoveredAsset := range discovered {
		asset := modelmap.CanonicalizeConnectorAsset(hubcollector.CollectorTypeDocker, discoveredAsset)
		metadata := make(map[string]string, len(asset.Metadata)+2)
		for key, value := range asset.Metadata {
			metadata[key] = value
		}
		resourceKind, metadata := WithCanonicalResourceMetadata("docker", asset.Type, metadata)

		assetType := strings.TrimSpace(asset.Kind)
		if assetType == "" {
			assetType = asset.Type
		}
		if strings.TrimSpace(resourceKind) != "" {
			assetType = strings.TrimSpace(resourceKind)
		}
		req := assets.HeartbeatRequest{
			AssetID:  asset.ID,
			Type:     assetType,
			Name:     asset.Name,
			Source:   "docker",
			Status:   NormalizeDockerStatus(metadata),
			Platform: "",
			Metadata: metadata,
		}
		if _, err := d.ProcessHeartbeatRequest(req); err != nil {
			log.Printf("hub collector docker: failed to upsert %s: %v", asset.ID, err)
			continue
		}
		snapshotAssets = append(snapshotAssets, ConnectorSnapshotAssetFromHeartbeat(req, asset.Kind))
		ingested++
	}

	if clusterAsset, ok := d.KeepConnectorClusterAssetAlive(collector, "docker", ingested, "hub collector docker"); ok {
		snapshotAssets = append(snapshotAssets, clusterAsset)
	}

	d.pruneStaleDockerDiscoveryAssets(discovered)
	d.PersistCanonicalConnectorSnapshot("docker", collector.ID, connector.DisplayName(), "", connector, snapshotAssets)

	if err := d.AutoLinkDockerHostsToInfra(); err != nil {
		log.Printf("hub collector docker: failed to auto-link runs_on chain: %v", err)
	}
	if err := d.AutoLinkDockerContainersToHosts(); err != nil {
		log.Printf("hub collector docker: failed to auto-link containers to hosts: %v", err)
	}

	if ingested == 0 {
		log.Printf("hub collector docker: no assets discovered (waiting for docker-enabled agent reports)")
	}

	d.UpdateCollectorStatus(collector.ID, "ok", "")
}

// pruneStaleDockerDiscoveryAssets deletes Docker assets that a reporting
// host no longer has, such as a removed container. A host whose agent is
// offline reports nothing, so its host, containers and stacks are kept (they
// read as offline) until the agent returns or the device is removed.
func (d *Deps) pruneStaleDockerDiscoveryAssets(discovered []connectorsdk.Asset) {
	if d == nil || d.AssetStore == nil {
		return
	}

	liveAssetIDs := make(map[string]struct{}, len(discovered))
	reportingAgents := make(map[string]struct{}, len(discovered))
	for _, asset := range discovered {
		assetID := strings.TrimSpace(asset.ID)
		if assetID == "" {
			continue
		}
		liveAssetIDs[assetID] = struct{}{}
		if agentID := strings.TrimSpace(asset.Metadata["agent_id"]); agentID != "" {
			reportingAgents[agentID] = struct{}{}
		}
	}

	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		log.Printf("hub collector docker: failed to list assets for stale prune: %v", err)
		return
	}

	for _, candidate := range assetList {
		if !isManagedDockerDiscoveryAsset(candidate) {
			continue
		}
		if _, ok := liveAssetIDs[strings.TrimSpace(candidate.ID)]; ok {
			continue
		}
		if agentID := strings.TrimSpace(candidate.Metadata["agent_id"]); agentID != "" {
			if _, reporting := reportingAgents[agentID]; !reporting {
				continue
			}
		}
		if err := d.AssetStore.DeleteAsset(candidate.ID); err != nil && !errors.Is(err, persistence.ErrNotFound) {
			log.Printf("hub collector docker: failed to delete stale asset %s: %v", candidate.ID, err)
		}
	}
}

func isManagedDockerDiscoveryAsset(assetEntry assets.Asset) bool {
	if !strings.EqualFold(strings.TrimSpace(assetEntry.Source), "docker") {
		return false
	}

	assetType := strings.TrimSpace(assetEntry.Type)
	switch assetType {
	case "container-host", "docker-container", "compose-stack":
		return true
	case "connector-cluster":
		return false
	}

	assetID := strings.ToLower(strings.TrimSpace(assetEntry.ID))
	return strings.HasPrefix(assetID, "docker-host-") ||
		strings.HasPrefix(assetID, "docker-ct-") ||
		strings.HasPrefix(assetID, "docker-stack-")
}

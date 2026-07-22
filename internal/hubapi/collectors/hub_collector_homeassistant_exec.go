package collectors

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/homeassistant"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
)

func (d *Deps) executeHomeAssistantCollector(ctx context.Context, collector hubcollector.Collector) {
	lifecycle := NewCollectorLifecycle(d, collector, "homeassistant", hubcollector.CollectorTypeHomeAssistant)
	failUnavailable := func(message string) {
		d.markHomeAssistantParentUnavailable(collector)
		lifecycle.Fail(message)
	}
	failUnavailablef := func(format string, args ...any) {
		failUnavailable(fmt.Sprintf(format, args...))
	}

	if d.CredentialStore == nil || d.SecretsManager == nil {
		failUnavailable("credential store unavailable")
		return
	}

	baseURL := CollectorConfigString(collector.Config, "base_url")
	credentialID := CollectorConfigString(collector.Config, "credential_id")
	if baseURL == "" {
		failUnavailable("missing base_url in config")
		return
	}
	if credentialID == "" {
		failUnavailable("missing credential_id in config")
		return
	}

	cred, ok, err := d.CredentialStore.GetCredentialProfile(credentialID)
	if err != nil || !ok {
		failUnavailable("credential not found")
		return
	}
	token, err := d.SecretsManager.DecryptString(cred.SecretCiphertext, cred.ID)
	if err != nil {
		failUnavailable("failed to decrypt credential")
		return
	}

	skipVerify, hasSkipVerify := collectorConfigBool(collector.Config, "skip_verify")
	if !hasSkipVerify {
		skipVerify = false
	}
	connector := homeassistant.NewWithConfig(homeassistant.Config{
		BaseURL:    baseURL,
		Token:      strings.TrimSpace(token),
		SkipVerify: skipVerify,
		Timeout:    collectorConfigDuration(collector.Config, "timeout", 15*time.Second),
	})

	collectorCtx, collectorCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer collectorCancel()

	discovered, err := connector.Discover(collectorCtx)
	if err != nil {
		failUnavailablef("home assistant discovery failed: %v", err)
		return
	}

	// Fetch HA config (version, location name) — best-effort.
	haConfig, configErr := connector.FetchConfig(collectorCtx)
	if configErr != nil {
		log.Printf("hub collector homeassistant: config fetch failed (non-fatal): %v", configErr)
	}

	// Fetch Supervisor host stats — best-effort, only available on HAOS/Supervised.
	supervisorStats, _ := connector.FetchSupervisorStats(collectorCtx)

	eligibleAssets := 0
	ingested := 0
	upsertFailures := 0
	snapshotAssets := make([]connectorsdk.Asset, 0)
	for _, discoveredAsset := range discovered {
		if strings.HasPrefix(discoveredAsset.ID, "ha-entity-sensor-labtemp") || strings.HasPrefix(discoveredAsset.ID, "ha-entity-switch-rack-fan") {
			continue
		}
		eligibleAssets++
		metadata := make(map[string]string)
		for key, value := range discoveredAsset.Metadata {
			metadata[key] = value
		}
		metadata["collector_id"] = strings.TrimSpace(collector.ID)
		metadata["collector_base_url"] = strings.TrimSpace(baseURL)
		resourceKind, metadata := WithCanonicalResourceMetadata("homeassistant", discoveredAsset.Type, metadata)

		assetType := strings.TrimSpace(discoveredAsset.Type)
		if strings.TrimSpace(resourceKind) != "" {
			assetType = strings.TrimSpace(resourceKind)
		}
		if assetType == "" {
			assetType = "ha-entity"
		}
		req := assets.HeartbeatRequest{
			AssetID:  discoveredAsset.ID,
			Type:     assetType,
			Name:     discoveredAsset.Name,
			Source:   "homeassistant",
			Status:   normalizeHomeAssistantStatus(metadata),
			Metadata: metadata,
		}
		req = ScopedCollectorHeartbeatRequest(collector.ID, req)
		if _, err := d.ProcessScopedCollectorHeartbeat(collector.ID, req); err != nil {
			log.Printf("hub collector homeassistant: failed to upsert %s: %v", discoveredAsset.ID, err)
			upsertFailures++
			continue
		}
		snapshotAssets = append(snapshotAssets, ConnectorSnapshotAssetFromHeartbeat(req, discoveredAsset.Kind))
		ingested++
	}

	clusterName := strings.TrimSpace(CollectorConfigString(collector.Config, "display_name"))
	if clusterName == "" && haConfig.LocationName != "" {
		clusterName = haConfig.LocationName
	}
	if clusterName == "" && d.AssetStore != nil {
		if existing, ok, err := d.AssetStore.GetAsset(collector.AssetID); err == nil && ok {
			clusterName = strings.TrimSpace(existing.Name)
		}
	}

	if clusterAsset, ok := d.RefreshCollectorParentAsset(CollectorParentAssetRefreshOptions{
		Collector: collector,
		Source:    "homeassistant",
		AssetType: "connector-cluster",
		Name:      clusterName,
		Status:    "online",
		Metadata: func() map[string]string {
			m := map[string]string{
				"connector_type":     "homeassistant",
				"collector_base_url": strings.TrimSpace(baseURL),
				"discovered":         fmt.Sprintf("%d", ingested),
			}
			if haConfig.Version != "" {
				m["ha_version"] = haConfig.Version
			}
			if haConfig.LocationName != "" {
				m["ha_location_name"] = haConfig.LocationName
			}
			if supervisorStats.Available {
				m["ha_cpu_percent"] = fmt.Sprintf("%.1f", supervisorStats.CPUPercent)
				m["ha_memory_used_percent"] = fmt.Sprintf("%.1f", supervisorStats.MemoryUsedPercent)
				m["ha_disk_used_percent"] = fmt.Sprintf("%.1f", supervisorStats.DiskUsedPercent)
			}
			if supervisorStats.OSName != "" {
				m["ha_os_name"] = supervisorStats.OSName
			}
			if supervisorStats.Hostname != "" {
				m["ha_host_name"] = supervisorStats.Hostname
			}
			return m
		}(),
		LogPrefix:      "hub collector homeassistant",
		FailureSubject: "cluster asset",
	}); ok {
		snapshotAssets = append(snapshotAssets, clusterAsset)
	}

	d.PersistCanonicalConnectorSnapshot("home-assistant", collector.ID, connector.DisplayName(), "", connector, snapshotAssets)

	if eligibleAssets == 0 {
		lifecycle.Partial("no assets discovered from Home Assistant")
		return
	}
	if ingested == 0 {
		lifecycle.Failf("failed to persist discovered Home Assistant assets: visible=%d upsert_failures=%d", eligibleAssets, upsertFailures)
		return
	}

	d.AppendConnectorLogEvent(
		collector.AssetID,
		"homeassistant",
		"info",
		fmt.Sprintf("collector run complete: discovered=%d", ingested),
		lifecycle.logFields,
		time.Now().UTC(),
	)

	if upsertFailures > 0 {
		lifecycle.Partialf("partial Home Assistant inventory persisted: assets=%d upsert_failures=%d", ingested, upsertFailures)
		return
	}

	d.UpdateCollectorStatus(collector.ID, "ok", "")
}

// markHomeAssistantParentUnavailable records connector reachability without
// replacing the last successful Home Assistant inventory. The parent heartbeat
// is refreshed as offline, while its existing name and metadata are carried
// forward so a failed poll cannot masquerade stale details as a fresh sync.
func (d *Deps) markHomeAssistantParentUnavailable(collector hubcollector.Collector) {
	assetID := strings.TrimSpace(collector.AssetID)
	if assetID == "" || d.AssetStore == nil || d.ProcessHeartbeatRequest == nil {
		return
	}

	name := strings.TrimSpace(CollectorConfigString(collector.Config, "display_name"))
	assetType := "connector-cluster"
	metadata := map[string]string{
		"connector_type": "homeassistant",
	}
	if baseURL := strings.TrimSpace(CollectorConfigString(collector.Config, "base_url")); baseURL != "" {
		metadata["collector_base_url"] = baseURL
	}

	existing, ok, err := d.AssetStore.GetAsset(assetID)
	if err != nil {
		// Do not risk replacing the last successful metadata when the read needed
		// to preserve it failed. The collector error is still recorded below.
		return
	}
	if ok {
		existingSource := strings.ToLower(strings.TrimSpace(existing.Source))
		if existingSource != "homeassistant" && existingSource != "home-assistant" {
			return
		}
		if existingName := strings.TrimSpace(existing.Name); existingName != "" {
			name = existingName
		}
		if existingType := strings.TrimSpace(existing.Type); existingType != "" {
			assetType = existingType
		}
		metadata = cloneStringMap(existing.Metadata)
		if metadata == nil {
			metadata = make(map[string]string)
		}
	}

	metadata["connector_type"] = "homeassistant"
	if _, ok := metadata["collector_base_url"]; !ok {
		if baseURL := strings.TrimSpace(CollectorConfigString(collector.Config, "base_url")); baseURL != "" {
			metadata["collector_base_url"] = baseURL
		}
	}
	if name == "" {
		name = assetID
	}

	d.RefreshCollectorParentAsset(CollectorParentAssetRefreshOptions{
		Collector:      collector,
		Source:         "homeassistant",
		AssetType:      assetType,
		Name:           name,
		Status:         "offline",
		Metadata:       metadata,
		LogPrefix:      "hub collector homeassistant",
		FailureSubject: "unavailable cluster asset",
	})
}

func normalizeHomeAssistantStatus(metadata map[string]string) string {
	state := strings.ToLower(strings.TrimSpace(metadata["state"]))
	switch state {
	case "", "on", "open", "home", "active", "playing", "heat", "cool":
		return "online"
	case "unavailable", "unknown", "offline", "disconnected":
		return "offline"
	default:
		return "online"
	}
}

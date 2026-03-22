package collectors

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
)

func (d *Deps) ExecuteTrueNASCollector(ctx context.Context, collector hubcollector.Collector) {
	if d.CredentialStore == nil || d.SecretsManager == nil {
		d.UpdateCollectorStatus(collector.ID, "error", "credential store unavailable")
		return
	}
	lifecycle := NewCollectorLifecycle(d, collector, "truenas", hubcollector.CollectorTypeTrueNAS)

	baseURL := CollectorConfigString(collector.Config, "base_url")
	credentialID := CollectorConfigString(collector.Config, "credential_id")
	if baseURL == "" {
		lifecycle.Fail("missing base_url in config")
		return
	}
	if credentialID == "" {
		lifecycle.Fail("missing credential_id in config")
		return
	}

	cred, ok, err := d.CredentialStore.GetCredentialProfile(credentialID)
	if err != nil || !ok {
		lifecycle.Fail("credential not found")
		return
	}
	apiKey, err := d.SecretsManager.DecryptString(cred.SecretCiphertext, cred.ID)
	if err != nil {
		lifecycle.Fail("failed to decrypt credential")
		return
	}

	skipVerify, hasSkipVerify := collectorConfigBool(collector.Config, "skip_verify")
	if !hasSkipVerify {
		skipVerify = false
	}

	connector := truenas.NewWithConfig(truenas.Config{
		BaseURL:    baseURL,
		APIKey:     strings.TrimSpace(apiKey),
		SkipVerify: skipVerify,
		Timeout:    collectorConfigDuration(collector.Config, "timeout", 15*time.Second),
	})

	collectorCtx, collectorCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer collectorCancel()

	discovered, err := connector.Discover(collectorCtx)
	if err != nil {
		lifecycle.Failf("truenas discovery failed: %v", err)
		return
	}

	eligibleAssets := 0
	ingested := 0
	upsertFailures := 0
	snapshotAssets := make([]connectorsdk.Asset, 0, len(discovered)+1)
	endpointHost, endpointIP := CollectorEndpointIdentity(baseURL)
	for _, asset := range discovered {
		// Skip the stub asset when no real assets found
		if asset.ID == "truenas-controller-stub" {
			continue
		}
		eligibleAssets++
		metadata := make(map[string]string, len(asset.Metadata)+4)
		for key, value := range asset.Metadata {
			metadata[key] = value
		}
		metadata["collector_id"] = strings.TrimSpace(collector.ID)
		metadata["collector_base_url"] = strings.TrimSpace(baseURL)
		if endpointHost != "" {
			metadata["collector_endpoint_host"] = endpointHost
		}
		if endpointIP != "" {
			metadata["collector_endpoint_ip"] = endpointIP
		}
		// Map TrueNAS-specific metadata keys to canonical names expected by the
		// frontend (System panel, device cards, etc.) while preserving originals.
		if asset.Type == "nas" {
			if v := metadata["cores"]; v != "" {
				metadata["cpu_cores_physical"] = v
			}
			if v := metadata["physmem"]; v != "" {
				metadata["memory_total_bytes"] = v
			}
			if v := metadata["model"]; v != "" {
				metadata["cpu_model"] = v
			}
		}
		_, metadata = WithCanonicalResourceMetadata("truenas", asset.Type, metadata)
		req := assets.HeartbeatRequest{
			AssetID:  asset.ID,
			Type:     asset.Type,
			Name:     asset.Name,
			Source:   "truenas",
			Status:   NormalizeTrueNASStatus(asset.Metadata),
			Platform: "",
			Metadata: metadata,
		}
		if _, err := d.ProcessHeartbeatRequest(req); err != nil {
			log.Printf("hub collector truenas: failed to upsert %s: %v", asset.ID, err)
			upsertFailures++
			continue
		}
		snapshotAssets = append(snapshotAssets, ConnectorSnapshotAssetFromHeartbeat(req, asset.Kind))
		ingested++
	}

	alertLogs := 0
	truenasClient := &truenas.Client{
		BaseURL:    strings.TrimSpace(baseURL),
		APIKey:     strings.TrimSpace(apiKey),
		SkipVerify: skipVerify,
		Timeout:    collectorConfigDuration(collector.Config, "timeout", 15*time.Second),
	}
	if runtimeClient, runtimeErr := d.LoadTrueNASRuntime(strings.TrimSpace(collector.ID)); runtimeErr != nil {
		log.Printf("hub collector truenas: failed to start subscription worker for %s: %v", collector.ID, runtimeErr)
	} else {
		d.EnsureTrueNASSubscriptionWorker(ctx, collector.ID, runtimeClient)
	}
	if ingestedAlerts, alertErr := d.IngestTrueNASAlertLogs(collectorCtx, truenasClient, collector.AssetID); alertErr != nil {
		log.Printf("hub collector truenas: failed to ingest alert logs: %v", alertErr)
	} else {
		alertLogs = ingestedAlerts
	}

	if clusterAsset, ok := d.KeepConnectorClusterAssetAlive(collector, "truenas", ingested, "hub collector truenas"); ok {
		snapshotAssets = append(snapshotAssets, clusterAsset)
	}

	if err := d.AutoLinkTrueNASHostsToProxmoxGuests(); err != nil {
		log.Printf("hub collector truenas: failed to auto-link runs_on chain: %v", err)
	}
	d.PersistCanonicalConnectorSnapshot("truenas", collector.ID, connector.DisplayName(), "", connector, snapshotAssets)

	if eligibleAssets == 0 {
		lifecycle.Partial("no assets discovered from TrueNAS")
		return
	}
	if ingested == 0 {
		lifecycle.Failf("failed to persist discovered TrueNAS assets: visible=%d upsert_failures=%d", eligibleAssets, upsertFailures)
		return
	}
	d.AppendConnectorLogEvent(
		collector.AssetID,
		"truenas",
		"info",
		fmt.Sprintf("collector run complete: discovered=%d alert_events=%d", ingested, alertLogs),
		lifecycle.logFields,
		time.Now().UTC(),
	)

	if upsertFailures > 0 {
		lifecycle.Partialf("partial TrueNAS inventory persisted: assets=%d upsert_failures=%d", ingested, upsertFailures)
		return
	}

	d.UpdateCollectorStatus(collector.ID, "ok", "")
}

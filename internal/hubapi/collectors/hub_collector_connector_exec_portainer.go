package collectors

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/portainer"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/modelmap"
)

func (d *Deps) ExecutePortainerCollector(ctx context.Context, collector hubcollector.Collector) {
	if d.CredentialStore == nil || d.SecretsManager == nil {
		d.UpdateCollectorStatus(collector.ID, "error", "credential store unavailable")
		return
	}
	lifecycle := NewCollectorLifecycle(d, collector, "portainer", hubcollector.CollectorTypePortainer)

	baseURL := CollectorConfigString(collector.Config, "base_url")
	clusterName := CollectorConfigString(collector.Config, "cluster_name")
	credentialID := CollectorConfigString(collector.Config, "credential_id")
	authMethod := strings.ToLower(strings.TrimSpace(CollectorConfigString(collector.Config, "auth_method")))
	if authMethod == "" {
		authMethod = "api_key"
	}

	if baseURL == "" {
		d.UpdateCollectorStatus(collector.ID, "error", "missing base_url in config")
		return
	}
	if credentialID == "" {
		d.UpdateCollectorStatus(collector.ID, "error", "missing credential_id in config")
		return
	}

	cred, ok, err := d.CredentialStore.GetCredentialProfile(credentialID)
	if err != nil || !ok {
		d.UpdateCollectorStatus(collector.ID, "error", "credential not found")
		return
	}

	secret, err := d.SecretsManager.DecryptString(cred.SecretCiphertext, cred.ID)
	if err != nil {
		d.UpdateCollectorStatus(collector.ID, "error", "failed to decrypt credential")
		return
	}

	skipVerify := true
	if value, has := collector.Config["skip_verify"]; has {
		skipVerify = strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", value))) == "true"
	}

	portainerConfig := portainer.Config{
		BaseURL:    baseURL,
		APIKey:     "",
		Username:   strings.TrimSpace(cred.Username),
		Password:   "",
		SkipVerify: skipVerify,
		Timeout:    collectorConfigDuration(collector.Config, "timeout", 15*time.Second),
	}

	if authMethod == "password" {
		portainerConfig.Password = strings.TrimSpace(secret)
		if portainerConfig.Username == "" {
			d.UpdateCollectorStatus(collector.ID, "error", "missing username for password auth")
			return
		}
		if portainerConfig.Password == "" {
			d.UpdateCollectorStatus(collector.ID, "error", "missing password for password auth")
			return
		}
	} else {
		portainerConfig.APIKey = strings.TrimSpace(secret)
		if portainerConfig.APIKey == "" {
			d.UpdateCollectorStatus(collector.ID, "error", "missing api key in credential")
			return
		}
	}

	client := portainer.NewClient(portainerConfig)
	connector := portainer.NewWithClient(client)
	if !client.IsConfigured() {
		d.UpdateCollectorStatus(collector.ID, "error", "portainer client is not configured")
		return
	}

	collectorCtx, collectorCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer collectorCancel()

	versionInfo, versionErr := client.GetVersion(collectorCtx)
	if versionErr != nil {
		log.Printf("hub collector portainer: failed to query version metadata: %v", versionErr)
	}

	discovered, err := connector.Discover(collectorCtx)
	if err != nil {
		d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("portainer discovery failed: %v", err))
		return
	}

	ingested := 0
	eligibleAssets := 0
	upsertFailures := 0
	snapshotAssets := make([]connectorsdk.Asset, 0)
	endpointHost, endpointIP := CollectorEndpointIdentity(baseURL)
	endpointAssetCount := 0
	type endpointInventory struct {
		containers int
		stacks     int
	}
	endpointInventoryByID := make(map[string]endpointInventory)
	stackContainerCountByEndpointAndName := make(map[string]int)
	for _, discoveredAsset := range discovered {
		endpointID := strings.TrimSpace(discoveredAsset.Metadata["endpoint_id"])
		if strings.EqualFold(strings.TrimSpace(discoveredAsset.Type), "container-host") {
			endpointAssetCount++
			if endpointID != "" {
				endpointInventoryByID[endpointID] = endpointInventoryByID[endpointID]
			}
			continue
		}
		if endpointID == "" {
			continue
		}

		inventory := endpointInventoryByID[endpointID]
		switch strings.ToLower(strings.TrimSpace(discoveredAsset.Type)) {
		case "container":
			inventory.containers++
			if stackName := strings.TrimSpace(discoveredAsset.Metadata["stack"]); stackName != "" {
				stackContainerCountByEndpointAndName[portainerStackInventoryKey(endpointID, stackName)]++
			}
		case "stack", "compose-stack":
			inventory.stacks++
		}
		endpointInventoryByID[endpointID] = inventory
	}

	for _, discoveredAsset := range discovered {
		asset := modelmap.CanonicalizeConnectorAsset(hubcollector.CollectorTypePortainer, discoveredAsset)
		if strings.EqualFold(asset.ID, "portainer-endpoint-stub") {
			continue
		}
		eligibleAssets++

		metadata := make(map[string]string)
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
		if endpointURL := strings.TrimSpace(metadata["url"]); endpointURL != "" {
			if host, ip := CollectorEndpointIdentity(endpointURL); host != "" {
				metadata["endpoint_host"] = host
				if ip != "" {
					metadata["endpoint_ip"] = ip
				}
			}
		}
		if endpointID := strings.TrimSpace(metadata["endpoint_id"]); endpointID != "" {
			inventory := endpointInventoryByID[endpointID]
			if asset.Type == "container-host" {
				metadata["portainer_container_count"] = fmt.Sprintf("%d", inventory.containers)
				metadata["portainer_stack_count"] = fmt.Sprintf("%d", inventory.stacks)
			}
			if asset.Type == "stack" || asset.Type == "compose-stack" {
				metadata["portainer_stack_container_count"] = fmt.Sprintf(
					"%d",
					stackContainerCountByEndpointAndName[portainerStackInventoryKey(endpointID, asset.Name)],
				)
			}
		}
		if version := strings.TrimSpace(versionInfo.ServerVersion); version != "" {
			metadata["portainer_version"] = version
		}
		if databaseVersion := strings.TrimSpace(versionInfo.DatabaseVersion); databaseVersion != "" {
			metadata["portainer_database_version"] = databaseVersion
		}
		if buildNumber := strings.TrimSpace(versionInfo.Build.BuildNumber); buildNumber != "" {
			metadata["portainer_build_number"] = buildNumber
		}

		if asset.Type == "container-host" && endpointAssetCount == 1 && strings.TrimSpace(clusterName) != "" {
			if originalName := strings.TrimSpace(asset.Name); originalName != "" && !strings.EqualFold(originalName, clusterName) {
				metadata["portainer_endpoint_name"] = originalName
			}
			asset.Name = strings.TrimSpace(clusterName)
			metadata["name"] = asset.Name
		}
		_, metadata = WithCanonicalResourceMetadata("portainer", asset.Type, metadata)

		req := assets.HeartbeatRequest{
			AssetID:  asset.ID,
			Type:     asset.Type,
			Name:     asset.Name,
			Source:   "portainer",
			Status:   NormalizePortainerStatus(metadata),
			Platform: "",
			Metadata: metadata,
		}
		if _, err := d.ProcessHeartbeatRequest(req); err != nil {
			log.Printf("hub collector portainer: failed to upsert %s: %v", asset.ID, err)
			upsertFailures++
			continue
		}
		snapshotAssets = append(snapshotAssets, ConnectorSnapshotAssetFromHeartbeat(req, asset.Kind))
		ingested++
	}

	if clusterAsset, ok := d.KeepConnectorClusterAssetAlive(collector, "portainer", ingested, "hub collector portainer"); ok {
		snapshotAssets = append(snapshotAssets, clusterAsset)
	}

	d.PersistCanonicalConnectorSnapshot("portainer", collector.ID, connector.DisplayName(), "", connector, snapshotAssets)

	if eligibleAssets == 0 {
		lifecycle.Partial("no assets discovered from Portainer")
		return
	}
	if ingested == 0 {
		lifecycle.Failf("failed to persist discovered Portainer assets: visible=%d upsert_failures=%d", eligibleAssets, upsertFailures)
		return
	}

	if err := d.AutoLinkPortainerHostsToTrueNASHosts(); err != nil {
		log.Printf("hub collector portainer: failed to auto-link runs_on chain: %v", err)
	}

	if upsertFailures > 0 {
		lifecycle.Partialf("partial Portainer inventory persisted: assets=%d upsert_failures=%d", ingested, upsertFailures)
		return
	}

	d.UpdateCollectorStatus(collector.ID, "ok", "")
}

func portainerStackInventoryKey(endpointID, stackName string) string {
	return strings.ToLower(strings.TrimSpace(endpointID)) + "|" + strings.ToLower(strings.TrimSpace(stackName))
}

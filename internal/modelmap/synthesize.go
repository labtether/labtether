package modelmap

import (
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/model"
)

func SynthesizeResourceRelationships(connectorID string, assets []connectorsdk.Asset) []model.ResourceRelationship {
	return synthesizeResourceRelationshipsAt(connectorID, assets, time.Now().UTC())
}

func synthesizeResourceRelationshipsAt(connectorID string, assets []connectorsdk.Asset, now time.Time) []model.ResourceRelationship {
	canonicalAssets := CanonicalizeConnectorAssets(connectorID, assets)
	if len(canonicalAssets) == 0 {
		return nil
	}

	sourceID := normalizeSourceID(connectorID)
	if sourceID == "" {
		sourceID = normalizeSourceID(canonicalAssets[0].Source)
	}

	proxmoxNodeByName := map[string]string{}
	portainerEndpointByID := map[string]string{}
	idByKindName := map[string]string{}
	truenasPoolByName := map[string]string{}
	truenasDatasetByName := map[string]string{}
	pbsRootID := ""
	truenasRootID := ""

	for _, asset := range canonicalAssets {
		kindNameKey := assetKindNameKey(asset.Kind, asset.Name)
		if kindNameKey != "" {
			idByKindName[kindNameKey] = asset.ID
		}

		switch normalizeSourceID(asset.Source) {
		case "proxmox":
			if asset.Kind == "hypervisor-node" {
				proxmoxNodeByName[strings.ToLower(strings.TrimSpace(asset.Name))] = asset.ID
			}
		case "portainer":
			if asset.Kind == "container-host" {
				endpointID := strings.TrimSpace(asset.Metadata["endpoint_id"])
				if endpointID != "" {
					portainerEndpointByID[endpointID] = asset.ID
				}
			}
		case "pbs":
			if asset.Kind == "storage-controller" && pbsRootID == "" {
				pbsRootID = asset.ID
			}
		case "truenas":
			if asset.Kind == "storage-controller" && truenasRootID == "" {
				truenasRootID = asset.ID
			}
			if asset.Kind == "storage-pool" {
				truenasPoolByName[strings.ToLower(strings.TrimSpace(asset.Name))] = asset.ID
			}
			if asset.Kind == "dataset" {
				truenasDatasetByName[strings.ToLower(strings.TrimSpace(asset.Name))] = asset.ID
			}
		}
	}

	relationships := make([]model.ResourceRelationship, 0, len(canonicalAssets))
	seen := make(map[string]struct{}, len(canonicalAssets))
	add := func(sourceAssetID, targetAssetID string, relType model.RelationshipType, criticality model.RelationshipCriticality, confidence int, evidence map[string]any) {
		sourceAssetID = strings.TrimSpace(sourceAssetID)
		targetAssetID = strings.TrimSpace(targetAssetID)
		if sourceAssetID == "" || targetAssetID == "" || sourceAssetID == targetAssetID {
			return
		}

		key := strings.ToLower(sourceAssetID) + "|" + string(relType) + "|" + strings.ToLower(targetAssetID)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}

		relationships = append(relationships, model.ResourceRelationship{
			ID:               "rel-" + normalizeRelationshipID(sourceAssetID+"-"+string(relType)+"-"+targetAssetID),
			SourceResourceID: sourceAssetID,
			TargetResourceID: targetAssetID,
			Type:             relType,
			Direction:        model.RelationshipDirectionDownstream,
			Criticality:      criticality,
			Inferred:         true,
			Confidence:       clampConfidence(confidence),
			Evidence:         cloneAnyMap(evidence),
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	}

	for _, asset := range canonicalAssets {
		assetSource := normalizeSourceID(asset.Source)
		if assetSource == "" {
			assetSource = sourceID
		}

		switch assetSource {
		case "proxmox":
			nodeName := strings.ToLower(strings.TrimSpace(asset.Metadata["node"]))
			if nodeName == "" {
				continue
			}
			nodeID := strings.TrimSpace(proxmoxNodeByName[nodeName])
			if nodeID == "" {
				continue
			}
			switch asset.Kind {
			case "vm", "container":
				add(asset.ID, nodeID, model.RelationshipRunsOn, model.RelationshipCriticalityHigh, 95, map[string]any{"source": "metadata.node"})
			case "storage-pool":
				add(asset.ID, nodeID, model.RelationshipManagedBy, model.RelationshipCriticalityMedium, 82, map[string]any{"source": "metadata.node"})
			}
		case "portainer":
			endpointID := strings.TrimSpace(asset.Metadata["endpoint_id"])
			if endpointID != "" {
				hostID := strings.TrimSpace(portainerEndpointByID[endpointID])
				switch asset.Kind {
				case "container", "stack", "compose-stack":
					add(asset.ID, hostID, model.RelationshipRunsOn, model.RelationshipCriticalityHigh, 92, map[string]any{"source": "metadata.endpoint_id"})
				}
			}

			if asset.Kind == "container" {
				stackName := strings.TrimSpace(asset.Metadata["stack"])
				if stackName == "" {
					continue
				}
				if stackID := idByKindName[assetKindNameKey("stack", stackName)]; stackID != "" {
					add(asset.ID, stackID, model.RelationshipMemberOf, model.RelationshipCriticalityMedium, 78, map[string]any{"source": "metadata.stack"})
					continue
				}
				if stackID := idByKindName[assetKindNameKey("compose-stack", stackName)]; stackID != "" {
					add(asset.ID, stackID, model.RelationshipMemberOf, model.RelationshipCriticalityMedium, 78, map[string]any{"source": "metadata.stack"})
				}
			}
		case "pbs":
			if asset.Kind == "datastore" && pbsRootID != "" {
				add(pbsRootID, asset.ID, model.RelationshipContains, model.RelationshipCriticalityMedium, 90, map[string]any{"source": "pbs.discovery"})
			}
		case "truenas":
			if truenasRootID != "" && asset.ID != truenasRootID {
				add(truenasRootID, asset.ID, model.RelationshipContains, model.RelationshipCriticalityMedium, 70, map[string]any{"source": "truenas.discovery"})
			}
			if asset.Kind == "dataset" {
				poolName := truenasPoolNameFromDataset(asset.Name)
				if poolName == "" {
					continue
				}
				if poolID := truenasPoolByName[strings.ToLower(poolName)]; poolID != "" {
					add(poolID, asset.ID, model.RelationshipContains, model.RelationshipCriticalityMedium, 88, map[string]any{"source": "dataset.name"})
				}
			}
			if asset.Kind == "share-smb" || asset.Kind == "share-nfs" {
				poolName, datasetName := truenasPoolDatasetFromPath(asset.Metadata["path"])
				if datasetName != "" {
					if datasetID := truenasDatasetByName[strings.ToLower(datasetName)]; datasetID != "" {
						add(asset.ID, datasetID, model.RelationshipDependsOn, model.RelationshipCriticalityMedium, 80, map[string]any{"source": "share.path"})
						continue
					}
				}
				if poolName != "" {
					if poolID := truenasPoolByName[strings.ToLower(poolName)]; poolID != "" {
						add(asset.ID, poolID, model.RelationshipDependsOn, model.RelationshipCriticalityMedium, 72, map[string]any{"source": "share.path"})
					}
				}
			}
		}
	}

	sort.Slice(relationships, func(i, j int) bool {
		left := relationships[i]
		right := relationships[j]
		if left.SourceResourceID == right.SourceResourceID {
			if left.TargetResourceID == right.TargetResourceID {
				return left.Type < right.Type
			}
			return left.TargetResourceID < right.TargetResourceID
		}
		return left.SourceResourceID < right.SourceResourceID
	})

	return relationships
}

func SynthesizeCapabilitySets(connector connectorsdk.Connector, assets []connectorsdk.Asset) []model.CapabilitySet {
	return synthesizeCapabilitySetsAt(connector, assets, time.Now().UTC())
}

func synthesizeCapabilitySetsAt(connector connectorsdk.Connector, assets []connectorsdk.Asset, now time.Time) []model.CapabilitySet {
	if connector == nil {
		return nil
	}

	actions := CanonicalizeActionDescriptors(connector.Actions())
	canonicalAssets := CanonicalizeConnectorAssets(connector.ID(), assets)
	connectorCaps := connector.Capabilities()

	sets := make([]model.CapabilitySet, 0, len(canonicalAssets)+1)
	providerCaps := synthesizeProviderCapabilitySpecs(connectorCaps, actions)
	sets = append(sets, model.CapabilitySet{
		SubjectType:  "provider",
		SubjectID:    connector.ID(),
		Capabilities: providerCaps,
		UpdatedAt:    now,
	})

	for _, asset := range canonicalAssets {
		caps := synthesizeResourceCapabilitySpecs(asset.Kind, connectorCaps, actions)
		if len(caps) == 0 {
			continue
		}
		sets = append(sets, model.CapabilitySet{
			SubjectType:  "resource",
			SubjectID:    asset.ID,
			Capabilities: caps,
			UpdatedAt:    now,
		})
	}

	return sets
}

func synthesizeProviderCapabilitySpecs(connectorCaps connectorsdk.Capabilities, actions []connectorsdk.ActionDescriptor) []model.CapabilitySpec {
	caps := map[string]model.CapabilitySpec{}
	add := func(spec model.CapabilitySpec) {
		existing, has := caps[spec.ID]
		if !has {
			caps[spec.ID] = spec
			return
		}
		existing.SupportsDryRun = existing.SupportsDryRun || spec.SupportsDryRun
		existing.RequiresTarget = existing.RequiresTarget || spec.RequiresTarget
		caps[spec.ID] = existing
	}

	add(model.CapabilitySpec{ID: "health.read", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA})
	if connectorCaps.DiscoverAssets {
		add(model.CapabilitySpec{ID: "inventory.discover", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA})
	}
	if connectorCaps.CollectMetrics {
		add(model.CapabilitySpec{ID: "telemetry.read", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA})
	}
	if connectorCaps.CollectEvents {
		add(model.CapabilitySpec{ID: "events.read", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA})
	}
	if connectorCaps.ExecuteActions {
		add(model.CapabilitySpec{ID: "system.action", Scope: model.CapabilityScopeAction, Stability: model.CapabilityStabilityGA})
	}

	for _, action := range actions {
		canonicalOp := CanonicalOperationID(firstNonEmptyString(action.CanonicalID, action.ID))
		capabilityID := operationCapabilityID(canonicalOp)
		if capabilityID == "" {
			continue
		}
		add(model.CapabilitySpec{
			ID:             capabilityID,
			Scope:          model.CapabilityScopeAction,
			Stability:      model.CapabilityStabilityGA,
			SupportsDryRun: action.SupportsDryRun,
			RequiresTarget: action.RequiresTarget,
		})
	}

	return sortedCapabilitySpecs(caps)
}

func synthesizeResourceCapabilitySpecs(kind string, connectorCaps connectorsdk.Capabilities, actions []connectorsdk.ActionDescriptor) []model.CapabilitySpec {
	kind = normalizeKindToken(kind)
	if kind == "" {
		return nil
	}

	caps := map[string]model.CapabilitySpec{}
	add := func(spec model.CapabilitySpec) {
		existing, has := caps[spec.ID]
		if !has {
			caps[spec.ID] = spec
			return
		}
		existing.SupportsDryRun = existing.SupportsDryRun || spec.SupportsDryRun
		existing.RequiresTarget = existing.RequiresTarget || spec.RequiresTarget
		caps[spec.ID] = existing
	}

	add(model.CapabilitySpec{ID: "health.read", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA})
	if connectorCaps.CollectMetrics {
		add(model.CapabilitySpec{ID: "telemetry.read", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA})
	}
	if connectorCaps.CollectEvents {
		add(model.CapabilitySpec{ID: "events.read", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA})
	}

	for _, action := range actions {
		canonicalOp := CanonicalOperationID(firstNonEmptyString(action.CanonicalID, action.ID))
		capabilityID := operationCapabilityID(canonicalOp)
		if capabilityID == "" {
			continue
		}
		if !actionAppliesToResource(action, canonicalOp, kind) {
			continue
		}
		add(model.CapabilitySpec{
			ID:             capabilityID,
			Scope:          model.CapabilityScopeAction,
			Stability:      model.CapabilityStabilityGA,
			SupportsDryRun: action.SupportsDryRun,
			RequiresTarget: action.RequiresTarget,
		})
	}

	return sortedCapabilitySpecs(caps)
}

func actionAppliesToResource(action connectorsdk.ActionDescriptor, canonicalOpID string, resourceKind string) bool {
	targetKinds := actionTargetKinds(action.ID, canonicalOpID)
	if len(targetKinds) == 0 {
		return false
	}
	_, matches := targetKinds[resourceKind]
	return matches
}

func actionTargetKinds(rawActionID, canonicalOpID string) map[string]struct{} {
	raw := strings.ToLower(strings.TrimSpace(rawActionID))
	canonical := strings.ToLower(strings.TrimSpace(canonicalOpID))

	// Preserve explicit connector contracts when available.
	switch {
	case strings.HasPrefix(raw, "vm."):
		return kindSet("vm")
	case strings.HasPrefix(raw, "ct."):
		return kindSet("container")
	case strings.HasPrefix(raw, "container."):
		return kindSet("container", "docker-container")
	case strings.HasPrefix(raw, "stack."):
		return kindSet("stack", "compose-stack")
	case strings.HasPrefix(raw, "service."):
		return kindSet("service")
	case strings.HasPrefix(raw, "app."):
		return kindSet("app")
	case strings.HasPrefix(raw, "datastore."):
		return kindSet("datastore")
	case raw == "pool.scrub":
		return kindSet("storage-pool")
	case raw == "smart.test":
		return kindSet("disk")
	case raw == "entity.toggle":
		return kindSet("ha-entity")
	}

	switch {
	case strings.HasPrefix(canonical, "workload."):
		return kindSet("vm", "container", "docker-container", "app", "pod", "deployment")
	case strings.HasPrefix(canonical, "snapshot."):
		return kindSet("vm", "container", "storage-pool", "datastore", "dataset")
	case strings.HasPrefix(canonical, "backup."):
		return kindSet("vm", "container", "datastore")
	case strings.HasPrefix(canonical, "storage.pool."):
		return kindSet("storage-pool")
	case strings.HasPrefix(canonical, "storage.datastore."):
		return kindSet("datastore")
	case strings.HasPrefix(canonical, "disk."):
		return kindSet("disk")
	case strings.HasPrefix(canonical, "service."):
		return kindSet("service")
	case strings.HasPrefix(canonical, "app."):
		return kindSet("app")
	case strings.HasPrefix(canonical, "container."):
		return kindSet("container", "docker-container")
	case strings.HasPrefix(canonical, "image."):
		return kindSet("container-host", "stack", "compose-stack")
	case strings.HasPrefix(canonical, "stack."):
		return kindSet("stack", "compose-stack")
	case strings.HasPrefix(canonical, "automation."):
		return kindSet("ha-entity")
	case canonical == "system.reboot":
		return kindSet("host", "hypervisor-node", "container-host", "storage-controller")
	default:
		return nil
	}
}

func operationCapabilityID(canonicalOpID string) string {
	canonical := strings.ToLower(strings.TrimSpace(canonicalOpID))
	switch {
	case strings.HasPrefix(canonical, "workload."):
		return "workload.action"
	case strings.HasPrefix(canonical, "snapshot."):
		return "snapshot.action"
	case strings.HasPrefix(canonical, "backup."), strings.HasPrefix(canonical, "storage.datastore."):
		return "backup.action"
	case strings.HasPrefix(canonical, "service."):
		return "service.action"
	case strings.HasPrefix(canonical, "app."):
		return "app.action"
	case strings.HasPrefix(canonical, "stack."):
		return "stack.action"
	case strings.HasPrefix(canonical, "image."):
		return "image.action"
	case strings.HasPrefix(canonical, "container."), strings.HasPrefix(canonical, "storage.pool."), strings.HasPrefix(canonical, "disk."), strings.HasPrefix(canonical, "system."), strings.HasPrefix(canonical, "automation."):
		return "system.action"
	default:
		return ""
	}
}

func sortedCapabilitySpecs(in map[string]model.CapabilitySpec) []model.CapabilitySpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.CapabilitySpec, 0, len(in))
	for _, spec := range in {
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func assetKindNameKey(kind, name string) string {
	normalizedKind := normalizeKindToken(kind)
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedKind == "" || normalizedName == "" {
		return ""
	}
	return normalizedKind + "|" + normalizedName
}

func truenasPoolNameFromDataset(datasetName string) string {
	parts := strings.Split(strings.TrimSpace(datasetName), "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func truenasPoolDatasetFromPath(path string) (poolName string, datasetName string) {
	trimmed := strings.TrimSpace(path)
	if !strings.HasPrefix(trimmed, "/mnt/") {
		return "", ""
	}
	relative := strings.TrimPrefix(trimmed, "/mnt/")
	segments := strings.Split(relative, "/")
	if len(segments) == 0 {
		return "", ""
	}
	pool := strings.TrimSpace(segments[0])
	if pool == "" {
		return "", ""
	}
	if len(segments) < 2 {
		return pool, ""
	}
	dataset := strings.TrimSpace(segments[1])
	if dataset == "" {
		return pool, ""
	}
	return pool, pool + "/" + dataset
}

func normalizeRelationshipID(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = kindSanitizePattern.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	if normalized == "" {
		return "edge"
	}
	return normalized
}

func clampConfidence(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func kindSet(kinds ...string) map[string]struct{} {
	if len(kinds) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(kinds))
	for _, kind := range kinds {
		normalized := normalizeKindToken(kind)
		if normalized == "" {
			continue
		}
		out[normalized] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

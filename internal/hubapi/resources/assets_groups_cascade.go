package resources

import (
	"errors"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

var infraHostTypeBySource = map[string]string{
	"proxmox":        "hypervisor-node",
	"truenas":        "nas",
	"pbs":            "storage-controller",
	"docker":         "container-host",
	"portainer":      "container-host",
	"homeassistant":  "connector-cluster",
	"home-assistant": "connector-cluster",
}

// CascadeAssetSiteToInfraChildren propagates a parent asset's group assignment
// to all infrastructure child assets attached to it. It is a no-op when the
// parent is not an infra host.
func (d *Deps) CascadeAssetSiteToInfraChildren(parent assets.Asset) error {
	if !isInfraHostAsset(parent) {
		return nil
	}

	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		return err
	}

	groupID := parent.GroupID
	updateReq := assets.UpdateRequest{GroupID: &groupID}

	for _, candidate := range assetList {
		if !infraChildAttachedToParent(parent, candidate) {
			continue
		}
		if _, err := d.AssetStore.UpdateAsset(candidate.ID, updateReq); err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				continue
			}
			return err
		}
	}

	return nil
}

// InfraChildAttachedToParent reports whether candidate is an infrastructure
// child asset attached to the given parent. Exported for use in heartbeat
// deletion logic that remains in cmd/labtether.
func InfraChildAttachedToParent(parent assets.Asset, candidate assets.Asset) bool {
	return infraChildAttachedToParent(parent, candidate)
}

// IsInfraHostAsset reports whether the asset is an infrastructure host (e.g.
// a hypervisor node, NAS, or Docker host). Exported for deletion logic.
func IsInfraHostAsset(assetEntry assets.Asset) bool {
	return isInfraHostAsset(assetEntry)
}

// NormalizeSource lowercases and trims a source string. Exported for callers
// that remain in cmd/labtether and use source-based routing.
func NormalizeSource(source string) string {
	return normalizeSource(source)
}

func infraChildAttachedToParent(parent assets.Asset, candidate assets.Asset) bool {
	if candidate.ID == parent.ID {
		return false
	}
	if normalizeSource(candidate.Source) != normalizeSource(parent.Source) {
		return false
	}
	if !isInfraChildAsset(candidate) {
		return false
	}
	if isCollectorClusterParent(parent) {
		parentCollectorID := strings.TrimSpace(parent.Metadata["collector_id"])
		candidateCollectorID := strings.TrimSpace(candidate.Metadata["collector_id"])
		return parentCollectorID != "" && strings.EqualFold(candidateCollectorID, parentCollectorID)
	}
	parentKey := infraHostParentKey(parent)
	if parentKey == "" {
		return false
	}
	childKey := infraChildParentKey(candidate)
	if childKey == "" {
		return false
	}
	return strings.EqualFold(childKey, parentKey)
}

func isInfraHostAsset(assetEntry assets.Asset) bool {
	if isCollectorClusterParent(assetEntry) {
		return true
	}

	source := normalizeSource(assetEntry.Source)
	hostType, ok := infraHostTypeBySource[source]
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(assetEntry.Type), hostType)
}

func isInfraChildAsset(assetEntry assets.Asset) bool {
	if strings.EqualFold(strings.TrimSpace(assetEntry.Type), "connector-cluster") {
		return false
	}

	source := normalizeSource(assetEntry.Source)
	hostType, ok := infraHostTypeBySource[source]
	assetType := strings.TrimSpace(assetEntry.Type)
	if ok && strings.EqualFold(assetType, hostType) {
		return false
	}
	if ok {
		return true
	}

	return source != "" && assetType != ""
}

func isCollectorClusterParent(assetEntry assets.Asset) bool {
	if !strings.EqualFold(strings.TrimSpace(assetEntry.Type), "connector-cluster") {
		return false
	}
	if normalizeSource(assetEntry.Source) == "docker" {
		return false
	}
	return strings.TrimSpace(assetEntry.Metadata["collector_id"]) != ""
}

func infraChildParentKey(assetEntry assets.Asset) string {
	switch normalizeSource(assetEntry.Source) {
	case "proxmox":
		return infraCollectorScopedKey(assetEntry.Metadata["collector_id"], assetEntry.Metadata["node"])
	case "portainer":
		return infraCollectorScopedKey(assetEntry.Metadata["collector_id"], assetEntry.Metadata["endpoint_id"])
	case "docker":
		if agentID := strings.TrimSpace(assetEntry.Metadata["agent_id"]); agentID != "" {
			return agentID
		}
		return "docker"
	case "truenas", "pbs", "homeassistant", "home-assistant":
		source := normalizeSource(assetEntry.Source)
		if collectorID := strings.TrimSpace(assetEntry.Metadata["collector_id"]); collectorID != "" {
			return infraCollectorScopedKey(collectorID, source)
		}
		return source
	default:
		source := normalizeSource(assetEntry.Source)
		if _, ok := infraHostTypeBySource[source]; ok {
			return source
		}
		return ""
	}
}

func infraHostParentKey(assetEntry assets.Asset) string {
	switch normalizeSource(assetEntry.Source) {
	case "proxmox":
		node := strings.TrimSpace(assetEntry.Metadata["node"])
		if node == "" {
			node = strings.TrimSpace(assetEntry.Name)
		}
		return infraCollectorScopedKey(assetEntry.Metadata["collector_id"], node)
	case "portainer":
		collectorID := strings.TrimSpace(assetEntry.Metadata["collector_id"])
		if endpointID := strings.TrimSpace(assetEntry.Metadata["endpoint_id"]); endpointID != "" {
			return infraCollectorScopedKey(collectorID, endpointID)
		}
		return infraCollectorScopedKey(collectorID, strings.TrimSpace(assetEntry.Name))
	case "docker":
		if agentID := strings.TrimSpace(assetEntry.Metadata["agent_id"]); agentID != "" {
			return agentID
		}
		return "docker"
	case "truenas", "pbs", "homeassistant", "home-assistant":
		source := normalizeSource(assetEntry.Source)
		if collectorID := strings.TrimSpace(assetEntry.Metadata["collector_id"]); collectorID != "" {
			return infraCollectorScopedKey(collectorID, source)
		}
		return source
	default:
		return normalizeSource(assetEntry.Source)
	}
}

func infraCollectorScopedKey(scope, key string) string {
	trimmedKey := strings.TrimSpace(key)
	if trimmedKey == "" {
		return ""
	}
	trimmedScope := strings.TrimSpace(scope)
	if trimmedScope == "" {
		return trimmedKey
	}
	return trimmedScope + "::" + trimmedKey
}

func normalizeSource(source string) string {
	return strings.ToLower(strings.TrimSpace(source))
}

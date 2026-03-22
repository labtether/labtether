package modelmap

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/metricschema"
	"github.com/labtether/labtether/internal/model"
)

var (
	kindSanitizePattern = regexp.MustCompile(`[^a-z0-9.\-]+`)
	globalKindAliases   = map[string]string{
		"nas": "storage-controller",
	}
	sourceKindAliases = map[string]map[string]string{
		"truenas": {
			"nas": "storage-controller",
		},
		"pbs": {
			"storage-pool": "datastore",
		},
		"home-assistant": {
			"entity": "ha-entity",
		},
		"homeassistant": {
			"entity": "ha-entity",
		},
	}
)

func CanonicalizeConnectorAssets(connectorID string, assets []connectorsdk.Asset) []connectorsdk.Asset {
	if len(assets) == 0 {
		return nil
	}
	out := make([]connectorsdk.Asset, 0, len(assets))
	for _, asset := range assets {
		out = append(out, CanonicalizeConnectorAsset(connectorID, asset))
	}
	return out
}

func CanonicalizeConnectorAsset(connectorID string, asset connectorsdk.Asset) connectorsdk.Asset {
	out := asset
	out.Metadata = cloneStringMap(asset.Metadata)
	out.Attributes = cloneAnyMap(asset.Attributes)
	out.ProviderData = cloneAnyMap(asset.ProviderData)

	if strings.TrimSpace(out.Source) == "" {
		out.Source = strings.TrimSpace(connectorID)
	}

	out.Kind = CanonicalResourceKind(out.Source, firstNonEmptyString(strings.TrimSpace(out.Kind), out.Type), out.Metadata)
	if strings.TrimSpace(out.Class) == "" {
		out.Class = string(model.ResourceClassForKind(out.Kind))
	}

	rawType := normalizeKindToken(out.Type)
	if rawType != "" && out.Kind != "" && !strings.EqualFold(rawType, out.Kind) {
		if out.ProviderData == nil {
			out.ProviderData = make(map[string]any, 1)
		}
		if _, exists := out.ProviderData["raw_type"]; !exists {
			out.ProviderData["raw_type"] = rawType
		}
	}

	if len(out.Attributes) == 0 {
		_, _, attrs := DeriveAssetCanonical(out.Source, out.Type, out.Metadata)
		out.Attributes = attrs
	}

	return out
}

func CanonicalResourceKind(source, assetType string, metadata map[string]string) string {
	sourceID := normalizeSourceID(source)

	explicit := normalizeKindToken(metadata["resource_kind"])
	if explicit != "" {
		return remapResourceKind(sourceID, explicit)
	}

	kind := normalizeKindToken(assetType)
	if kind == "" {
		kind = metadataKindHint(sourceID, metadata)
	}
	if kind == "" {
		kind = "unknown"
	}
	return remapResourceKind(sourceID, kind)
}

func DeriveAssetCanonical(source, assetType string, metadata map[string]string) (resourceClass string, resourceKind string, attributes map[string]any) {
	kind := CanonicalResourceKind(source, assetType, metadata)

	class := strings.ToLower(strings.TrimSpace(metadata["resource_class"]))
	if class == "" {
		class = string(model.ResourceClassForKind(kind))
	}

	attrs := make(map[string]any, 20)
	attrs["source"] = strings.TrimSpace(source)

	copyStringAttr(attrs, metadata, "node")
	copyStringAttr(attrs, metadata, "vmid")
	copyStringAttr(attrs, metadata, "endpoint_id")
	copyStringAttr(attrs, metadata, "entity_id")
	copyStringAttr(attrs, metadata, "domain")
	copyStringAttr(attrs, metadata, "container_id")
	copyStringAttr(attrs, metadata, "stack")
	copyStringAttr(attrs, metadata, "stack_id")
	copyStringAttr(attrs, metadata, "image")
	copyStringAttr(attrs, metadata, "state")
	copyStringAttr(attrs, metadata, "status")
	copyStringAttr(attrs, metadata, "hostname")
	copyStringAttr(attrs, metadata, "version")
	copyStringAttr(attrs, metadata, "model")
	copyStringAttr(attrs, metadata, "service")
	copyStringAttr(attrs, metadata, "storage_id")
	copyStringAttr(attrs, metadata, "store")
	copyStringAttr(attrs, metadata, "pool_id")
	copyStringAttr(attrs, metadata, "path")
	copyStringAttr(attrs, metadata, "mountpoint")
	copyStringAttr(attrs, metadata, "url")
	copyStringAttr(attrs, metadata, "guest_uuid")
	copyStringAttr(attrs, metadata, "guest_primary_ip")
	copyStringAttr(attrs, metadata, "guest_primary_mac")

	copyFloatAttr(attrs, metadata, "cpu_used_percent", metricschema.HeartbeatKeyCPUUsedPercent, metricschema.HeartbeatKeyCPUPercent, "cpu_percent")
	copyFloatAttr(attrs, metadata, "memory_used_percent", metricschema.HeartbeatKeyMemoryUsedPercent, metricschema.HeartbeatKeyMemoryPercent, "memory_percent")
	copyFloatAttr(attrs, metadata, "disk_used_percent", metricschema.HeartbeatKeyDiskUsedPercent, metricschema.HeartbeatKeyDiskPercent, "disk_percent", "usage_percent")
	copyFloatAttr(attrs, metadata, "temperature_celsius", metricschema.HeartbeatKeyTemperatureCelsius, metricschema.HeartbeatKeyTempCelsius)
	copyFloatAttr(attrs, metadata, "network_rx_bytes_per_sec", metricschema.HeartbeatKeyNetworkRXBytesPerSec)
	copyFloatAttr(attrs, metadata, "network_tx_bytes_per_sec", metricschema.HeartbeatKeyNetworkTXBytesPerSec)
	copyFloatAttr(attrs, metadata, "total_bytes", "total_bytes", "size_bytes")
	copyFloatAttr(attrs, metadata, "used_bytes", "used_bytes", "alloc_bytes")
	copyFloatAttr(attrs, metadata, "available_bytes", "avail_bytes", "free_bytes")
	copyFloatAttr(attrs, metadata, "uptime_seconds", "uptime_sec")

	if len(attrs) == 1 { // source only
		attrs = nil
	}

	return class, kind, attrs
}

func normalizeSourceID(source string) string {
	return strings.ToLower(strings.TrimSpace(source))
}

func normalizeKindToken(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return ""
	}
	normalized = strings.ReplaceAll(normalized, "_", "-")
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = kindSanitizePattern.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	return normalized
}

func remapResourceKind(source, kind string) string {
	normalized := normalizeKindToken(kind)
	if normalized == "" {
		return ""
	}
	if aliases, ok := sourceKindAliases[source]; ok {
		if mapped, ok := aliases[normalized]; ok {
			return mapped
		}
	}
	if mapped, ok := globalKindAliases[normalized]; ok {
		return mapped
	}
	return normalized
}

func metadataKindHint(source string, metadata map[string]string) string {
	switch source {
	case "pbs":
		if strings.TrimSpace(metadata["store"]) != "" {
			return "datastore"
		}
	case "truenas":
		if strings.TrimSpace(metadata["pool_id"]) != "" {
			return "storage-pool"
		}
		if strings.TrimSpace(metadata["mountpoint"]) != "" {
			return "dataset"
		}
	case "portainer":
		switch {
		case strings.TrimSpace(metadata["container_id"]) != "":
			return "container"
		case strings.TrimSpace(metadata["stack_id"]) != "":
			return "stack"
		case strings.TrimSpace(metadata["endpoint_id"]) != "":
			return "container-host"
		}
	case "home-assistant", "homeassistant":
		if strings.TrimSpace(metadata["entity_id"]) != "" {
			return "ha-entity"
		}
	}
	if strings.TrimSpace(metadata["entity_id"]) != "" {
		return "ha-entity"
	}
	return ""
}

func copyStringAttr(out map[string]any, metadata map[string]string, key string) {
	value := strings.TrimSpace(metadata[key])
	if value == "" {
		return
	}
	out[key] = value
}

func copyFloatAttr(out map[string]any, metadata map[string]string, attrKey string, keys ...string) {
	for _, key := range keys {
		raw := strings.TrimSpace(metadata[key])
		if raw == "" {
			continue
		}
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		out[attrKey] = parsed
		return
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneAnyValue(value)
	}
	return out
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		out := make([]any, len(typed))
		for idx := range typed {
			out[idx] = cloneAnyValue(typed[idx])
		}
		return out
	case map[string]string:
		return cloneStringMap(typed)
	case []string:
		out := make([]string, len(typed))
		copy(out, typed)
		return out
	default:
		return value
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

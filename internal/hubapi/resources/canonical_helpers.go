package resources

// canonical_helpers.go — pure functions for mapping assets to the canonical
// model. These functions are stateless and have no dependencies on apiServer
// or any store. They are kept here so that both cmd/labtether and the alerting
// evaluator can import them without circular dependencies.

import (
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/model"
	"github.com/labtether/labtether/internal/modelregistry"
)

// CanonicalResourceFromAsset maps an assets.Asset to a model.Resource.
func CanonicalResourceFromAsset(assetEntry assets.Asset, providerInstanceID string) model.Resource {
	return model.Resource{
		ID:                 strings.TrimSpace(assetEntry.ID),
		Class:              model.ResourceClass(strings.TrimSpace(assetEntry.ResourceClass)),
		Kind:               canonicalFirstNonEmpty(assetEntry.ResourceKind, assetEntry.Type),
		Name:               strings.TrimSpace(assetEntry.Name),
		Source:             CanonicalProviderName(assetEntry.Source),
		ProviderInstanceID: strings.TrimSpace(providerInstanceID),
		GroupID:            strings.TrimSpace(assetEntry.GroupID),
		Platform:           strings.TrimSpace(assetEntry.Platform),
		Status:             CanonicalResourceStatus(assetEntry.Status),
		Attributes:         shared.CloneAnyMap(assetEntry.Attributes),
		FirstSeenAt:        assetEntry.CreatedAt.UTC(),
		LastSeenAt:         assetEntry.LastSeenAt.UTC(),
		UpdatedAt:          assetEntry.UpdatedAt.UTC(),
	}
}

// CanonicalExternalRefsFromAsset builds the external ref slice for a heartbeat asset.
func CanonicalExternalRefsFromAsset(assetEntry assets.Asset, providerInstanceID string) []model.ExternalRef {
	resourceID := strings.TrimSpace(assetEntry.ID)
	if resourceID == "" {
		return nil
	}
	reference := model.ExternalRef{
		ProviderInstanceID: strings.TrimSpace(providerInstanceID),
		ExternalID:         CanonicalExternalID(resourceID, assetEntry.Metadata),
		ExternalType:       canonicalFirstNonEmpty(assetEntry.ResourceKind, assetEntry.Type),
		ExternalParentID:   CanonicalExternalParentID(assetEntry.Metadata),
		RawLocator:         canonicalFirstNonEmpty(assetEntry.Metadata["url"], assetEntry.Metadata["path"]),
	}
	if reference.ProviderInstanceID == "" || reference.ExternalID == "" {
		return nil
	}
	return []model.ExternalRef{reference}
}

// CanonicalExternalID returns the best external identifier from metadata,
// falling back to the asset's own ID.
func CanonicalExternalID(fallback string, metadata map[string]string) string {
	keys := []string{
		"external_id",
		"proxmox_id",
		"entity_id",
		"container_id",
		"stack_id",
		"endpoint_id",
		"storage_id",
		"pool_id",
		"store",
		"vmid",
	}
	for _, key := range keys {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			return value
		}
	}
	return strings.TrimSpace(fallback)
}

// CanonicalExternalParentID returns the best external parent identifier from metadata.
func CanonicalExternalParentID(metadata map[string]string) string {
	keys := []string{"external_parent_id", "node", "endpoint_id", "pool_id"}
	for _, key := range keys {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			return value
		}
	}
	return ""
}

// InferCapabilityIDsFromAssetMetadata derives capability IDs from asset metadata signals.
func InferCapabilityIDsFromAssetMetadata(assetEntry assets.Asset) []string {
	metadata := assetEntry.Metadata
	if len(metadata) == 0 {
		return nil
	}

	capabilitySet := map[string]struct{}{
		"health.read": {},
	}
	if HasAnyMetricSignals(metadata) {
		capabilitySet["telemetry.read"] = struct{}{}
	}

	for _, token := range SplitCapabilityTokens(metadata["cap_services"]) {
		switch token {
		case "list":
			capabilitySet["service.list"] = struct{}{}
		case "action":
			capabilitySet["service.action"] = struct{}{}
		}
	}
	for _, token := range SplitCapabilityTokens(metadata["cap_packages"]) {
		switch token {
		case "list":
			capabilitySet["package.list"] = struct{}{}
		case "action":
			capabilitySet["package.action"] = struct{}{}
		}
	}
	for _, token := range SplitCapabilityTokens(metadata["cap_network"]) {
		switch token {
		case "list":
			capabilitySet["network.list"] = struct{}{}
		case "action":
			capabilitySet["network.action"] = struct{}{}
		}
	}
	for _, token := range SplitCapabilityTokens(metadata["cap_schedules"]) {
		switch token {
		case "list":
			capabilitySet["cron.list"] = struct{}{}
		}
	}
	for _, token := range SplitCapabilityTokens(metadata["cap_logs"]) {
		switch token {
		case "stored":
			capabilitySet["logs.read"] = struct{}{}
		case "query":
			capabilitySet["logs.query"] = struct{}{}
		case "stream":
			capabilitySet["logs.stream"] = struct{}{}
		}
	}

	source := CanonicalProviderName(assetEntry.Source)
	if IsAgentSource(source) {
		capabilitySet["terminal.open"] = struct{}{}
		capabilitySet["process.list"] = struct{}{}
		platform := strings.ToLower(strings.TrimSpace(canonicalFirstNonEmpty(assetEntry.Platform, metadata["platform"])))
		if platform == "linux" {
			capabilitySet["files.list"] = struct{}{}
			capabilitySet["users.list"] = struct{}{}
		}
	}

	out := make([]string, 0, len(capabilitySet))
	for capabilityID := range capabilitySet {
		out = append(out, capabilityID)
	}
	sort.Strings(out)
	return out
}

// CapabilitySpecsFromIDs converts a list of capability ID strings to CapabilitySpec values
// using the model registry catalog.
func CapabilitySpecsFromIDs(ids []string) []model.CapabilitySpec {
	if len(ids) == 0 {
		return nil
	}
	specByID := make(map[string]model.CapabilitySpec, len(modelregistry.CapabilityCatalog()))
	for _, spec := range modelregistry.CapabilityCatalog() {
		specByID[spec.ID] = spec
	}
	out := make([]model.CapabilitySpec, 0, len(ids))
	for _, id := range ids {
		normalized := strings.ToLower(strings.TrimSpace(id))
		if normalized == "" {
			continue
		}
		if descriptor, ok := specByID[normalized]; ok {
			out = append(out, descriptor)
			continue
		}
		out = append(out, model.CapabilitySpec{ID: normalized, Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityExperimental})
	}
	return out
}

// CapabilityIDsFromSet extracts the normalized capability ID strings from a CapabilitySet.
func CapabilityIDsFromSet(set model.CapabilitySet) []string {
	if len(set.Capabilities) == 0 {
		return nil
	}
	out := make([]string, 0, len(set.Capabilities))
	for _, capability := range set.Capabilities {
		normalized := strings.ToLower(strings.TrimSpace(capability.ID))
		if normalized == "" {
			continue
		}
		out = append(out, normalized)
	}
	return DedupeSortedStrings(out)
}

// MergeCapabilityIDs merges multiple capability ID slices into a deduplicated sorted slice.
func MergeCapabilityIDs(values ...[]string) []string {
	set := make(map[string]struct{}, 16)
	for _, list := range values {
		for _, value := range list {
			normalized := strings.ToLower(strings.TrimSpace(value))
			if normalized == "" {
				continue
			}
			set[normalized] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// SplitCapabilityTokens splits a comma-separated capability token string into
// normalized individual tokens.
func SplitCapabilityTokens(value string) []string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(value)), ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		out = append(out, token)
	}
	return out
}

// HasAnyMetricSignals returns true if the metadata contains any known metric keys.
func HasAnyMetricSignals(metadata map[string]string) bool {
	keys := []string{
		"cpu_used_percent",
		"memory_used_percent",
		"disk_used_percent",
		"temperature_celsius",
		"network_rx_bytes_per_sec",
		"network_tx_bytes_per_sec",
	}
	for _, key := range keys {
		if strings.TrimSpace(metadata[key]) != "" {
			return true
		}
	}
	return false
}

// CanonicalProviderName normalizes a provider/source name to its canonical form.
func CanonicalProviderName(source string) string {
	normalized := strings.ToLower(strings.TrimSpace(source))
	switch normalized {
	case "homeassistant":
		return "home-assistant"
	default:
		return normalized
	}
}

// CanonicalProviderInstanceID builds a stable provider instance ID.
func CanonicalProviderInstanceID(kind model.ProviderKind, provider, instanceKey string) string {
	provider = CanonicalProviderName(provider)
	instanceKey = strings.TrimSpace(instanceKey)
	if kind == model.ProviderKindAgent {
		if instanceKey == "" {
			instanceKey = provider
		}
		return "prov-agent-" + shared.NormalizeAssetKey(instanceKey)
	}
	if instanceKey == "" {
		return "prov-connector-" + shared.NormalizeAssetKey(provider)
	}
	return "prov-connector-" + shared.NormalizeAssetKey(provider) + "-" + shared.NormalizeAssetKey(instanceKey)
}

// ProviderStatusFromAssetStatus maps an asset status string to a ProviderStatus.
func ProviderStatusFromAssetStatus(status string) model.ProviderStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "online", "ok", "healthy":
		return model.ProviderStatusHealthy
	case "stale", "degraded", "warning":
		return model.ProviderStatusDegraded
	case "offline", "down", "error", "failed":
		return model.ProviderStatusOffline
	default:
		return model.ProviderStatusUnknown
	}
}

// CanonicalResourceStatus maps an asset status string to a ResourceStatus.
func CanonicalResourceStatus(status string) model.ResourceStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "online", "ok", "healthy":
		return model.ResourceStatusOnline
	case "stale":
		return model.ResourceStatusStale
	case "degraded", "warning":
		return model.ResourceStatusDegraded
	case "offline", "down", "error", "failed":
		return model.ResourceStatusOffline
	default:
		return model.ResourceStatusUnknown
	}
}

// ProviderScopeForGroup returns the provider scope based on whether a group is set.
func ProviderScopeForGroup(groupID string) model.ProviderScope {
	if strings.TrimSpace(groupID) != "" {
		return model.ProviderScopeGroup
	}
	return model.ProviderScopeGlobal
}

// IsAgentSource returns true if the normalized source string indicates an agent provider.
func IsAgentSource(source string) bool {
	source = CanonicalProviderName(source)
	return source == "agent"
}

// DedupeSortedStrings returns a deduplicated, sorted copy of the input slice.
func DedupeSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sort.Strings(values)
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if len(out) > 0 && out[len(out)-1] == value {
			continue
		}
		out = append(out, value)
	}
	return out
}

// canonicalFirstNonEmpty returns the first non-empty string from the arguments.
// This is a package-local helper; callers that need it in cmd/labtether should
// continue using their existing firstNonEmptyString alias.
func canonicalFirstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// CanonicalProviderInstanceForHeartbeat builds a ProviderInstance from a heartbeat request.
// The at parameter is the timestamp to record as LastSeenAt.
func CanonicalProviderInstanceForHeartbeat(
	assetEntry assets.Asset,
	source string,
	at time.Time,
) model.ProviderInstance {
	source = CanonicalProviderName(source)
	kind := model.ProviderKindConnector
	instanceKey := strings.TrimSpace(assetEntry.Metadata["collector_id"])
	if IsAgentSource(source) {
		kind = model.ProviderKindAgent
		instanceKey = strings.TrimSpace(assetEntry.ID)
	}
	instanceID := CanonicalProviderInstanceID(kind, source, instanceKey)
	displayName := strings.TrimSpace(assetEntry.Name)
	if displayName == "" {
		displayName = source
	}

	metadata := map[string]any{}
	if endpointHost := strings.TrimSpace(assetEntry.Metadata["collector_endpoint_host"]); endpointHost != "" {
		metadata["endpoint_host"] = endpointHost
	}
	if endpointIP := strings.TrimSpace(assetEntry.Metadata["collector_endpoint_ip"]); endpointIP != "" {
		metadata["endpoint_ip"] = endpointIP
	}
	if instanceKey != "" {
		metadata["instance_key"] = instanceKey
	}

	return model.ProviderInstance{
		ID:          instanceID,
		Kind:        kind,
		Provider:    source,
		DisplayName: displayName,
		Version:     strings.TrimSpace(assetEntry.Metadata["version"]),
		Status:      ProviderStatusFromAssetStatus(assetEntry.Status),
		Scope:       ProviderScopeForGroup(assetEntry.GroupID),
		ConfigRef:   instanceKey,
		Metadata:    metadata,
		LastSeenAt:  at,
	}
}

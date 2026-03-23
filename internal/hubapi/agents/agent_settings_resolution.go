package agents

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"fmt"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"io"
	"net/url"
	"os"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentsettings"
	"github.com/labtether/labtether/internal/runtimesettings"
)

const AgentSettingsStorePrefix = "agent.settings."

type AgentSettingEntry struct {
	Key             string   `json:"key"`
	Label           string   `json:"label"`
	Description     string   `json:"description"`
	Type            string   `json:"type"`
	DefaultValue    string   `json:"default_value"`
	GlobalValue     string   `json:"global_value,omitempty"`
	OverrideValue   string   `json:"override_value,omitempty"`
	StateValue      string   `json:"state_value,omitempty"`
	EffectiveValue  string   `json:"effective_value"`
	Source          string   `json:"source"`
	MinInt          int      `json:"min_int,omitempty"`
	MaxInt          int      `json:"max_int,omitempty"`
	AllowedValues   []string `json:"allowed_values,omitempty"`
	RestartRequired bool     `json:"restart_required,omitempty"`
	HubManaged      bool     `json:"hub_managed"`
	LocalOnly       bool     `json:"local_only"`
	Drift           bool     `json:"drift,omitempty"`
}

type AgentSettingsViewState struct {
	Status               string            `json:"status"`
	Revision             string            `json:"revision,omitempty"`
	LastError            string            `json:"last_error,omitempty"`
	UpdatedAt            string            `json:"updated_at,omitempty"`
	AppliedAt            string            `json:"applied_at,omitempty"`
	RestartRequired      bool              `json:"restart_required,omitempty"`
	AllowRemoteOverrides bool              `json:"allow_remote_overrides"`
	Fingerprint          string            `json:"fingerprint,omitempty"`
	Values               map[string]string `json:"values,omitempty"`
}

type AgentSettingsPayload struct {
	AssetID                string                  `json:"asset_id"`
	Connected              bool                    `json:"connected"`
	Fingerprint            string                  `json:"fingerprint,omitempty"`
	AgentVersion           string                  `json:"agent_version,omitempty"`
	LatestAgentVersion     string                  `json:"latest_agent_version,omitempty"`
	LatestAgentPublishedAt string                  `json:"latest_agent_published_at,omitempty"`
	AgentVersionStatus     string                  `json:"agent_version_status,omitempty"`
	AgentVersionError      string                  `json:"agent_version_error,omitempty"`
	AgentPlatform          string                  `json:"agent_platform,omitempty"`
	AgentArch              string                  `json:"agent_arch,omitempty"`
	Settings               []AgentSettingEntry     `json:"settings"`
	State                  *AgentSettingsViewState `json:"state,omitempty"`
}

func AgentSettingStoreKey(assetID, key string) string {
	return AgentSettingsStorePrefix + url.QueryEscape(assetID) + "." + key
}

func AgentSettingStorePrefixForAsset(assetID string) string {
	return AgentSettingsStorePrefix + url.QueryEscape(assetID) + "."
}

func NormalizeAgentSettingValues(values map[string]string, forHubApply bool) (map[string]string, error) {
	out := make(map[string]string, len(values))
	for rawKey, rawValue := range values {
		key := strings.TrimSpace(rawKey)
		definition, ok := agentsettings.AgentSettingDefinitionByKey(key)
		if !ok {
			return nil, fmt.Errorf("unknown setting key: %s", key)
		}
		if forHubApply && definition.LocalOnly {
			return nil, fmt.Errorf("setting %s is local-only", key)
		}
		normalized, err := agentsettings.NormalizeAgentSettingValue(key, rawValue)
		if err != nil {
			return nil, err
		}
		out[key] = normalized
	}
	return out, nil
}

func (d *Deps) CollectEffectiveAgentSettingValues(assetID string) (map[string]string, error) {
	definitions := agentsettings.AgentSettingDefinitions()
	effective := agentsettings.DefaultAgentSettingValues()
	if d.RuntimeStore == nil {
		return effective, nil
	}

	overrides, err := d.RuntimeStore.ListRuntimeSettingOverrides()
	if err != nil {
		return nil, err
	}
	for _, definition := range definitions {
		if globalKey, ok := AgentSettingGlobalDefaultKey(definition.Key); ok {
			if raw, ok := overrides[globalKey]; ok {
				if normalized, err := agentsettings.NormalizeAgentSettingValue(definition.Key, raw); err == nil {
					effective[definition.Key] = normalized
				}
			}
		}
		assetKey := AgentSettingStoreKey(assetID, definition.Key)
		if raw, ok := overrides[assetKey]; ok {
			if normalized, err := agentsettings.NormalizeAgentSettingValue(definition.Key, raw); err == nil {
				effective[definition.Key] = normalized
			}
		}
	}
	return effective, nil
}

func (d *Deps) BuildAgentSettingsPayload(assetID string) (AgentSettingsPayload, error) {
	payload := AgentSettingsPayload{
		AssetID:   assetID,
		Connected: d.AgentMgr != nil && d.AgentMgr.IsConnected(assetID),
		Settings:  []AgentSettingEntry{},
	}

	var assetFingerprint string
	var agentVersion string
	var agentPlatform string
	var agentArch string
	if d.AssetStore != nil {
		asset, ok, err := d.AssetStore.GetAsset(assetID)
		if err != nil {
			return payload, err
		}
		if ok {
			assetFingerprint = strings.TrimSpace(asset.Metadata["agent_device_fingerprint"])
			agentVersion = strings.TrimSpace(asset.Metadata["agent_version"])
			agentPlatform = strings.TrimSpace(asset.Platform)
			if agentPlatform == "" {
				agentPlatform = strings.TrimSpace(asset.Metadata["platform"])
			}
			agentArch = strings.TrimSpace(asset.Metadata["cpu_architecture"])
		}
	}
	payload.Fingerprint = assetFingerprint

	if d.AgentMgr != nil {
		if conn, ok := d.AgentMgr.Get(assetID); ok {
			if agentVersion == "" {
				agentVersion = strings.TrimSpace(conn.Meta("agent_version"))
			}
			if agentPlatform == "" {
				agentPlatform = strings.TrimSpace(conn.Platform)
			}
		}
	}

	payload.AgentVersion = agentVersion
	payload.AgentPlatform = agentPlatform
	payload.AgentArch = agentArch

	releaseOS := NormalizeAgentReleaseOS(agentPlatform)
	releaseArch := NormalizeAgentReleaseArch(agentArch)
	if releaseOS == "darwin" && releaseArch == "" {
		// Current release endpoint requires arch for all OS values.
		releaseArch = "amd64"
	}
	if releaseOS != "" && releaseArch != "" {
		latestVersion, publishedAt, err := d.LatestAgentVersionForPlatform(releaseOS, releaseArch)
		if err != nil {
			payload.AgentVersionError = err.Error()
		} else {
			payload.LatestAgentVersion = latestVersion
			payload.LatestAgentPublishedAt = publishedAt
		}
	} else {
		payload.AgentVersionError = "platform/architecture not available for release lookup"
	}
	payload.AgentVersionStatus = DetermineAgentVersionStatus(payload.AgentVersion, payload.LatestAgentVersion)

	definitions := agentsettings.AgentSettingDefinitions()
	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Key < definitions[j].Key
	})

	effectiveValues, err := d.CollectEffectiveAgentSettingValues(assetID)
	if err != nil {
		return payload, err
	}

	state, hasState := d.GetAgentSettingsRuntimeState(assetID)
	stateValues := map[string]string{}
	if hasState && state.Values != nil {
		stateValues = state.Values
	}

	overrides := map[string]string{}
	global := map[string]string{}
	if d.RuntimeStore != nil {
		all, err := d.RuntimeStore.ListRuntimeSettingOverrides()
		if err == nil {
			prefix := AgentSettingStorePrefixForAsset(assetID)
			for key, value := range all {
				if strings.HasPrefix(key, prefix) {
					settingKey := strings.TrimPrefix(key, prefix)
					overrides[settingKey] = value
				}
			}
			for _, definition := range definitions {
				if globalKey, ok := AgentSettingGlobalDefaultKey(definition.Key); ok {
					if value, exists := all[globalKey]; exists {
						global[definition.Key] = value
					}
				}
			}
		}
	}

	for _, definition := range definitions {
		effectiveValue := effectiveValues[definition.Key]
		source := "default"
		overrideValue := ""
		globalValue := ""
		if raw, ok := global[definition.Key]; ok {
			if normalized, err := agentsettings.NormalizeAgentSettingValue(definition.Key, raw); err == nil {
				globalValue = normalized
				source = "hub-default"
			}
		}
		if raw, ok := overrides[definition.Key]; ok {
			if normalized, err := agentsettings.NormalizeAgentSettingValue(definition.Key, raw); err == nil {
				overrideValue = normalized
				source = "hub-override"
			}
		}

		stateValue := ""
		stateValueKnown := false
		if raw, ok := stateValues[definition.Key]; ok {
			stateValue = strings.TrimSpace(raw)
			stateValueKnown = true
		}
		drift := stateValueKnown && stateValue != effectiveValue
		entry := AgentSettingEntry{
			Key:             definition.Key,
			Label:           definition.Label,
			Description:     definition.Description,
			Type:            string(definition.Type),
			DefaultValue:    definition.DefaultValue,
			GlobalValue:     globalValue,
			OverrideValue:   overrideValue,
			StateValue:      stateValue,
			EffectiveValue:  effectiveValue,
			Source:          source,
			MinInt:          definition.MinInt,
			MaxInt:          definition.MaxInt,
			AllowedValues:   append([]string(nil), definition.AllowedValues...),
			RestartRequired: definition.RestartRequired,
			HubManaged:      definition.HubManaged,
			LocalOnly:       definition.LocalOnly,
			Drift:           drift,
		}
		payload.Settings = append(payload.Settings, entry)
	}

	if hasState {
		payload.State = &AgentSettingsViewState{
			Status:               state.Status,
			Revision:             state.Revision,
			LastError:            state.LastError,
			UpdatedAt:            ZeroTimeToRFC3339(state.UpdatedAt),
			AppliedAt:            ZeroTimeToRFC3339(state.AppliedAt),
			RestartRequired:      state.RestartRequired,
			AllowRemoteOverrides: state.AllowRemoteOverrides,
			Fingerprint:          state.Fingerprint,
			Values:               CloneAgentSettingValues(state.Values),
		}
	}
	return payload, nil
}

func (d *Deps) LatestAgentVersionForPlatform(agentOS, arch string) (string, string, error) {
	_, binaryPath, err := ResolveAgentBinaryPath(d.AgentBinaryDir, agentOS, arch)
	if err != nil {
		return "", "", err
	}

	// #nosec G304 -- constrained by ResolveAgentBinaryPath allowlist.
	f, err := os.Open(binaryPath)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", "", err
	}

	version := strings.TrimSpace(shared.EnvOrDefault("LABTETHER_AGENT_RELEASE_VERSION", ""))
	if version == "" {
		version = DetectAgentBinaryVersion(binaryPath)
	}
	if version == "" {
		sum := sha256.New()
		if _, err := io.Copy(sum, f); err != nil {
			return "", "", err
		}
		sha := hex.EncodeToString(sum.Sum(nil))
		if len(sha) < 12 {
			return "", "", fmt.Errorf("invalid release hash")
		}
		version = "sha256:" + sha[:12]
	}

	return version, info.ModTime().UTC().Format(time.RFC3339), nil
}

func DetermineAgentVersionStatus(currentVersion, latestVersion string) string {
	current := strings.TrimSpace(currentVersion)
	latest := strings.TrimSpace(latestVersion)
	if current == "" || latest == "" {
		return "unknown"
	}
	if strings.EqualFold(current, latest) {
		return "up_to_date"
	}
	return "update_available"
}

func NormalizeAgentReleaseOS(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "linux":
		return "linux"
	case "darwin", "mac", "macos":
		return "darwin"
	case "windows", "win32":
		return "windows"
	default:
		return ""
	}
}

func NormalizeAgentReleaseArch(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "amd64", "x86_64", "x64":
		return "amd64"
	case "arm64", "aarch64":
		return "arm64"
	default:
		return ""
	}
}

func DetectAgentBinaryVersion(binaryPath string) string {
	info, err := buildinfo.ReadFile(binaryPath)
	if err != nil || info == nil {
		return ""
	}

	return AgentVersionFromBuildInfo(info.Main.Version, info.Settings)
}

func AgentVersionFromBuildInfo(mainVersion string, settings []debug.BuildSetting) string {
	version := strings.TrimSpace(mainVersion)
	if version != "" && version != "(devel)" {
		return version
	}

	revision := ""
	modified := false
	for _, setting := range settings {
		switch strings.TrimSpace(setting.Key) {
		case "vcs.revision":
			revision = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			modified = strings.EqualFold(strings.TrimSpace(setting.Value), "true")
		}
	}

	if revision == "" {
		return ""
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified {
		revision += "-dirty"
	}
	return "git:" + revision
}

func AgentSettingGlobalDefaultKey(key string) (string, bool) {
	switch key {
	case agentsettings.SettingKeyCollectIntervalSec:
		return "agent_collect_interval_sec", true
	case agentsettings.SettingKeyHeartbeatIntervalSec:
		return "agent_heartbeat_interval_sec", true
	case agentsettings.SettingKeyServicesDiscoveryDockerEnabled:
		return runtimesettings.KeyServicesDiscoveryDefaultDockerEnabled, true
	case agentsettings.SettingKeyServicesDiscoveryProxyEnabled:
		return runtimesettings.KeyServicesDiscoveryDefaultProxyEnabled, true
	case agentsettings.SettingKeyServicesDiscoveryProxyTraefikEnabled:
		return runtimesettings.KeyServicesDiscoveryDefaultProxyTraefikEnabled, true
	case agentsettings.SettingKeyServicesDiscoveryProxyCaddyEnabled:
		return runtimesettings.KeyServicesDiscoveryDefaultProxyCaddyEnabled, true
	case agentsettings.SettingKeyServicesDiscoveryProxyNPMEnabled:
		return runtimesettings.KeyServicesDiscoveryDefaultProxyNPMEnabled, true
	case agentsettings.SettingKeyServicesDiscoveryPortScanEnabled:
		return runtimesettings.KeyServicesDiscoveryDefaultPortScanEnabled, true
	case agentsettings.SettingKeyServicesDiscoveryPortScanIncludeListening:
		return runtimesettings.KeyServicesDiscoveryDefaultPortScanIncludeListening, true
	case agentsettings.SettingKeyServicesDiscoveryPortScanPorts:
		return runtimesettings.KeyServicesDiscoveryDefaultPortScanPorts, true
	case agentsettings.SettingKeyServicesDiscoveryLANScanEnabled:
		return runtimesettings.KeyServicesDiscoveryDefaultLANScanEnabled, true
	case agentsettings.SettingKeyServicesDiscoveryLANScanCIDRs:
		return runtimesettings.KeyServicesDiscoveryDefaultLANScanCIDRs, true
	case agentsettings.SettingKeyServicesDiscoveryLANScanPorts:
		return runtimesettings.KeyServicesDiscoveryDefaultLANScanPorts, true
	case agentsettings.SettingKeyServicesDiscoveryLANScanMaxHosts:
		return runtimesettings.KeyServicesDiscoveryDefaultLANScanMaxHosts, true
	default:
		return "", false
	}
}

func CloneAgentSettingValues(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func ZeroTimeToRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

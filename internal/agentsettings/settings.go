// Package agentsettings contains hub-local copies of agent setting definitions,
// validation logic, and key constants. This allows the hub to manage agent
// settings without depending on the extracted agent packages.
package agentsettings

import (
	"fmt"
	"strconv"
	"strings"
)

// AgentSettingType defines the value type of an agent setting.
type AgentSettingType string

const (
	AgentSettingTypeString AgentSettingType = "string"
	AgentSettingTypeInt    AgentSettingType = "int"
	AgentSettingTypeBool   AgentSettingType = "bool"
	AgentSettingTypeEnum   AgentSettingType = "enum"
)

// Setting key constants.
const (
	SettingKeyCollectIntervalSec                        = "collect_interval_sec"
	SettingKeyHeartbeatIntervalSec                      = "heartbeat_interval_sec"
	SettingKeyDockerEnabled                             = "docker_enabled"
	SettingKeyDockerEndpoint                            = "docker_endpoint"
	SettingKeyDockerDiscoveryIntervalSec                = "docker_discovery_interval_sec"
	SettingKeyServicesDiscoveryDockerEnabled            = "services_discovery_docker_enabled"
	SettingKeyServicesDiscoveryProxyEnabled             = "services_discovery_proxy_enabled"
	SettingKeyServicesDiscoveryProxyTraefikEnabled      = "services_discovery_proxy_traefik_enabled"
	SettingKeyServicesDiscoveryProxyCaddyEnabled        = "services_discovery_proxy_caddy_enabled"
	SettingKeyServicesDiscoveryProxyNPMEnabled          = "services_discovery_proxy_npm_enabled"
	SettingKeyServicesDiscoveryPortScanEnabled          = "services_discovery_port_scan_enabled"
	SettingKeyServicesDiscoveryPortScanIncludeListening = "services_discovery_port_scan_include_listening"
	SettingKeyServicesDiscoveryPortScanPorts            = "services_discovery_port_scan_ports"
	SettingKeyServicesDiscoveryLANScanEnabled           = "services_discovery_lan_scan_enabled"
	SettingKeyServicesDiscoveryLANScanCIDRs             = "services_discovery_lan_scan_cidrs"
	SettingKeyServicesDiscoveryLANScanPorts             = "services_discovery_lan_scan_ports"
	SettingKeyServicesDiscoveryLANScanMaxHosts          = "services_discovery_lan_scan_max_hosts"
	SettingKeyFilesRootMode                             = "files_root_mode"
	SettingKeyWebRTCEnabled                             = "webrtc_enabled"
	SettingKeyWebRTCSTUNURL                             = "webrtc_stun_url"
	SettingKeyWebRTCTURNURL                             = "webrtc_turn_url"
	SettingKeyWebRTCTURNUser                            = "webrtc_turn_user"
	SettingKeyWebRTCTURNPass                            = "webrtc_turn_pass"
	SettingKeyWebRTCWaylandExperimentalEnabled          = "webrtc_wayland_experimental_enabled"
	SettingKeyWebRTCWaylandPipeWireNodeID               = "webrtc_wayland_pipewire_node_id"
	SettingKeyWebRTCWaylandInputBackend                 = "webrtc_wayland_input_backend"
	SettingKeyCaptureFPS                                = "capture_fps"
	SettingKeyAllowRemoteOverrides                      = "allow_remote_overrides"
	SettingKeyLogLevel                                  = "log_level"
	SettingKeyTLSSkipVerify                             = "tls_skip_verify"
	SettingKeyTLSCAFile                                 = "tls_ca_file"
)

// AgentSettingDefinition describes a single agent setting.
type AgentSettingDefinition struct {
	Key             string
	Label           string
	Description     string
	Type            AgentSettingType
	DefaultValue    string
	MinInt          int
	MaxInt          int
	AllowedValues   []string
	RestartRequired bool
	HubManaged      bool
	LocalOnly       bool
	Sensitive       bool
}

var agentSettingDefinitions = []AgentSettingDefinition{
	{
		Key:          SettingKeyCollectIntervalSec,
		Label:        "Metrics interval",
		Description:  "Seconds between metric samples.",
		Type:         AgentSettingTypeInt,
		DefaultValue: "10",
		MinInt:       2,
		MaxInt:       300,
		HubManaged:   true,
	},
	{
		Key:          SettingKeyHeartbeatIntervalSec,
		Label:        "Heartbeat interval",
		Description:  "Seconds between heartbeats.",
		Type:         AgentSettingTypeInt,
		DefaultValue: "20",
		MinInt:       5,
		MaxInt:       600,
		HubManaged:   true,
	},
	{
		Key:             SettingKeyDockerEnabled,
		Label:           "Docker",
		Description:     "auto: use Docker when it is installed. true or false: always on or off.",
		Type:            AgentSettingTypeEnum,
		DefaultValue:    "auto",
		AllowedValues:   []string{"auto", "true", "false"},
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyDockerEndpoint,
		Label:           "Docker socket",
		Description:     "Path or URL of the Docker API.",
		Type:            AgentSettingTypeString,
		DefaultValue:    "/var/run/docker.sock",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyDockerDiscoveryIntervalSec,
		Label:           "Docker refresh",
		Description:     "Seconds between container and stats updates.",
		Type:            AgentSettingTypeInt,
		DefaultValue:    "30",
		MinInt:          5,
		MaxInt:          3600,
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryDockerEnabled,
		Label:           "Services from Docker",
		Description:     "Find services from container ports and labels.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryProxyEnabled,
		Label:           "Services from reverse proxies",
		Description:     "Read routed services from the proxy APIs below.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryProxyTraefikEnabled,
		Label:           "Traefik routes",
		Description:     "",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryProxyCaddyEnabled,
		Label:           "Caddy routes",
		Description:     "From the Caddy admin API.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryProxyNPMEnabled,
		Label:           "Nginx Proxy Manager routes",
		Description:     "",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryPortScanEnabled,
		Label:           "Local port scan",
		Description:     "Probe this device's ports for services not found through Docker or a proxy.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryPortScanIncludeListening,
		Label:           "Add listening ports",
		Description:     "Also scan every TCP port this device listens on.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryPortScanPorts,
		Label:           "Local scan ports",
		Description:     "Comma or newline separated. Empty uses the built-in list.",
		Type:            AgentSettingTypeString,
		DefaultValue:    "",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryLANScanEnabled,
		Label:           "LAN scan",
		Description:     "Probe the LAN ranges for web services.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "false",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryLANScanCIDRs,
		Label:           "LAN ranges",
		Description:     "Private CIDRs, comma or newline separated.",
		Type:            AgentSettingTypeString,
		DefaultValue:    "",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryLANScanPorts,
		Label:           "LAN scan ports",
		Description:     "Comma or newline separated. Empty uses the built-in list.",
		Type:            AgentSettingTypeString,
		DefaultValue:    "",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryLANScanMaxHosts,
		Label:           "Hosts per LAN scan",
		Description:     "",
		Type:            AgentSettingTypeInt,
		DefaultValue:    "64",
		MinInt:          1,
		MaxInt:          1024,
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyFilesRootMode,
		Label:           "Files access",
		Description:     "home: the agent's home folder. full: the whole file system.",
		Type:            AgentSettingTypeEnum,
		DefaultValue:    "home",
		AllowedValues:   []string{"home", "full"},
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyWebRTCEnabled,
		Label:           "WebRTC",
		Description:     "Stream Remote View over WebRTC when the device supports it.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:          SettingKeyWebRTCSTUNURL,
		Label:        "STUN server",
		Description:  "",
		Type:         AgentSettingTypeString,
		DefaultValue: "stun:stun.l.google.com:19302",
		HubManaged:   true,
	},
	{
		Key:          SettingKeyWebRTCTURNURL,
		Label:        "TURN server",
		Description:  "Used when a direct connection fails.",
		Type:         AgentSettingTypeString,
		DefaultValue: "",
		HubManaged:   true,
	},
	{
		Key:          SettingKeyWebRTCTURNUser,
		Label:        "TURN username",
		Description:  "",
		Type:         AgentSettingTypeString,
		DefaultValue: "",
		HubManaged:   true,
	},
	{
		Key:          SettingKeyWebRTCTURNPass,
		Label:        "TURN password",
		Description:  "",
		Type:         AgentSettingTypeString,
		DefaultValue: "",
		HubManaged:   true,
		Sensitive:    true,
	},
	{
		Key:             SettingKeyWebRTCWaylandExperimentalEnabled,
		Label:           "Wayland desktop (experimental)",
		Description:     "Stream the real Wayland desktop on Linux.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "false",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyWebRTCWaylandPipeWireNodeID,
		Label:           "PipeWire node ID",
		Description:     "For the Wayland desktop stream.",
		Type:            AgentSettingTypeString,
		DefaultValue:    "",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyWebRTCWaylandInputBackend,
		Label:           "Wayland input",
		Description:     "How keyboard and mouse reach the Wayland desktop.",
		Type:            AgentSettingTypeEnum,
		DefaultValue:    "auto",
		AllowedValues:   []string{"auto", "none", "ydotool"},
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:          SettingKeyCaptureFPS,
		Label:        "Frame rate",
		Description:  "Frames per second for Remote View over WebRTC.",
		Type:         AgentSettingTypeInt,
		DefaultValue: "30",
		MinInt:       5,
		MaxInt:       120,
		HubManaged:   true,
	},
	{
		Key:             SettingKeyTLSSkipVerify,
		Label:           "Skip TLS verification",
		Description:     "For first setup only. The agent does not check the hub's certificate.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "false",
		RestartRequired: true,
		LocalOnly:       true,
	},
	{
		Key:             SettingKeyTLSCAFile,
		Label:           "Hub CA file",
		Description:     "PEM file the agent uses to check the hub's certificate.",
		Type:            AgentSettingTypeString,
		DefaultValue:    "",
		RestartRequired: true,
		LocalOnly:       true,
	},
	{
		Key:          SettingKeyAllowRemoteOverrides,
		Label:        "Allow hub changes",
		Description:  "Settings changed in the hub apply on this device.",
		Type:         AgentSettingTypeBool,
		DefaultValue: "true",
		LocalOnly:    true,
	},
	{
		Key:           SettingKeyLogLevel,
		Label:         "Log level",
		Description:   "",
		Type:          AgentSettingTypeEnum,
		DefaultValue:  "info",
		AllowedValues: []string{"debug", "info", "warn", "error"},
		HubManaged:    true,
	},
}

var agentSettingDefinitionsByKey = buildAgentSettingDefinitionsByKey(agentSettingDefinitions)

func buildAgentSettingDefinitionsByKey(items []AgentSettingDefinition) map[string]AgentSettingDefinition {
	out := make(map[string]AgentSettingDefinition, len(items))
	for _, item := range items {
		out[item.Key] = item
	}
	return out
}

// AgentSettingDefinitions returns a copy of all setting definitions.
func AgentSettingDefinitions() []AgentSettingDefinition {
	out := make([]AgentSettingDefinition, len(agentSettingDefinitions))
	copy(out, agentSettingDefinitions)
	return out
}

// AgentSettingDefinitionByKey returns the definition for the given key.
func AgentSettingDefinitionByKey(key string) (AgentSettingDefinition, bool) {
	item, ok := agentSettingDefinitionsByKey[strings.TrimSpace(strings.ToLower(key))]
	return item, ok
}

// NormalizeAgentSettingValue validates and normalizes a setting value.
func NormalizeAgentSettingValue(key, raw string) (string, error) {
	definition, ok := AgentSettingDefinitionByKey(key)
	if !ok {
		return "", fmt.Errorf("unknown agent setting key: %s", key)
	}
	value := strings.TrimSpace(raw)
	switch definition.Type {
	case AgentSettingTypeString:
		if definition.Key == SettingKeyDockerEndpoint {
			return normalizeDockerEndpointValue(value)
		}
		if definition.Key == SettingKeyWebRTCTURNURL ||
			definition.Key == SettingKeyWebRTCTURNUser ||
			definition.Key == SettingKeyWebRTCTURNPass ||
			definition.Key == SettingKeyWebRTCWaylandPipeWireNodeID ||
			definition.Key == SettingKeyTLSCAFile {
			return value, nil
		}
		if definition.Key == SettingKeyServicesDiscoveryPortScanPorts ||
			definition.Key == SettingKeyServicesDiscoveryLANScanPorts {
			return NormalizeDiscoveryPortListValue(definition.Key, value)
		}
		if definition.Key == SettingKeyServicesDiscoveryLANScanCIDRs {
			return NormalizeDiscoveryCIDRListValue(definition.Key, value)
		}
		if value == "" {
			return "", fmt.Errorf("%s cannot be empty", definition.Key)
		}
		return value, nil
	case AgentSettingTypeInt:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return "", fmt.Errorf("%s must be a number", definition.Key)
		}
		if definition.MinInt > 0 && parsed < definition.MinInt {
			return "", fmt.Errorf("%s must be >= %d", definition.Key, definition.MinInt)
		}
		if definition.MaxInt > 0 && parsed > definition.MaxInt {
			return "", fmt.Errorf("%s must be <= %d", definition.Key, definition.MaxInt)
		}
		return strconv.Itoa(parsed), nil
	case AgentSettingTypeBool:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return "", fmt.Errorf("%s must be true or false", definition.Key)
		}
		return strconv.FormatBool(parsed), nil
	case AgentSettingTypeEnum:
		for _, allowed := range definition.AllowedValues {
			if strings.EqualFold(value, allowed) {
				return allowed, nil
			}
		}
		return "", fmt.Errorf("%s must be one of: %s", definition.Key, strings.Join(definition.AllowedValues, ", "))
	default:
		return "", fmt.Errorf("unsupported agent setting type for %s", definition.Key)
	}
}

// DefaultAgentSettingValues returns a map of setting keys to their default values.
func DefaultAgentSettingValues() map[string]string {
	out := make(map[string]string, len(agentSettingDefinitions))
	for _, definition := range agentSettingDefinitions {
		out[definition.Key] = definition.DefaultValue
	}
	return out
}

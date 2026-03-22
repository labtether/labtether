package sysconfig

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"

	"github.com/labtether/labtether/internal/securityruntime"
)

type AgentSettingType string

const (
	AgentSettingTypeString AgentSettingType = "string"
	AgentSettingTypeInt    AgentSettingType = "int"
	AgentSettingTypeBool   AgentSettingType = "bool"
	AgentSettingTypeEnum   AgentSettingType = "enum"
)

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
}

var agentSettingDefinitions = []AgentSettingDefinition{
	{
		Key:          SettingKeyCollectIntervalSec,
		Label:        "Collect Interval",
		Description:  "Telemetry collection interval in seconds.",
		Type:         AgentSettingTypeInt,
		DefaultValue: "10",
		MinInt:       2,
		MaxInt:       300,
		HubManaged:   true,
	},
	{
		Key:          SettingKeyHeartbeatIntervalSec,
		Label:        "Heartbeat Interval",
		Description:  "Heartbeat publish interval in seconds.",
		Type:         AgentSettingTypeInt,
		DefaultValue: "20",
		MinInt:       5,
		MaxInt:       600,
		HubManaged:   true,
	},
	{
		Key:             SettingKeyDockerEnabled,
		Label:           "Docker Enabled",
		Description:     "Docker collector mode: auto, enabled, or disabled.",
		Type:            AgentSettingTypeEnum,
		DefaultValue:    "auto",
		AllowedValues:   []string{"auto", "true", "false"},
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyDockerEndpoint,
		Label:           "Docker Endpoint",
		Description:     "Docker endpoint path or URL.",
		Type:            AgentSettingTypeString,
		DefaultValue:    "/var/run/docker.sock",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyDockerDiscoveryIntervalSec,
		Label:           "Docker Discovery Interval",
		Description:     "Docker discovery/stats interval in seconds.",
		Type:            AgentSettingTypeInt,
		DefaultValue:    "30",
		MinInt:          5,
		MaxInt:          3600,
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryDockerEnabled,
		Label:           "Services: Docker Discovery Enabled",
		Description:     "When enabled, discover services from Docker container metadata and ports.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryProxyEnabled,
		Label:           "Services: Proxy API Discovery Enabled",
		Description:     "When enabled, query supported reverse-proxy APIs for routed services.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryProxyTraefikEnabled,
		Label:           "Services: Traefik API Discovery",
		Description:     "Include Traefik API routes in service discovery.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryProxyCaddyEnabled,
		Label:           "Services: Caddy API Discovery",
		Description:     "Include Caddy admin API routes in service discovery.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryProxyNPMEnabled,
		Label:           "Services: Nginx Proxy Manager API Discovery",
		Description:     "Include Nginx Proxy Manager API routes in service discovery.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryPortScanEnabled,
		Label:           "Services: Local Port Scan Enabled",
		Description:     "When enabled, probe local host ports for services not found via Docker/proxy APIs.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryPortScanIncludeListening,
		Label:           "Services: Local Scan Include Listening Ports",
		Description:     "Augment configured local scan ports with currently listening TCP sockets.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryPortScanPorts,
		Label:           "Services: Local Scan Port List",
		Description:     "Optional comma/newline separated local port list. Empty uses built-in defaults.",
		Type:            AgentSettingTypeString,
		DefaultValue:    "",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryLANScanEnabled,
		Label:           "Services: LAN CIDR Scan Enabled",
		Description:     "When enabled, probe configured CIDR ranges for web services.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "false",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryLANScanCIDRs,
		Label:           "Services: LAN CIDR Ranges",
		Description:     "Comma/newline separated private CIDRs to scan when LAN scan is enabled.",
		Type:            AgentSettingTypeString,
		DefaultValue:    "",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryLANScanPorts,
		Label:           "Services: LAN Scan Port List",
		Description:     "Optional comma/newline separated LAN port list. Empty uses built-in defaults.",
		Type:            AgentSettingTypeString,
		DefaultValue:    "",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyServicesDiscoveryLANScanMaxHosts,
		Label:           "Services: LAN Scan Host Cap",
		Description:     "Maximum number of LAN hosts probed per scan cycle.",
		Type:            AgentSettingTypeInt,
		DefaultValue:    "64",
		MinInt:          1,
		MaxInt:          1024,
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyFilesRootMode,
		Label:           "Files Root Mode",
		Description:     "File browser scope: home (sandboxed) or full (entire filesystem).",
		Type:            AgentSettingTypeEnum,
		DefaultValue:    "home",
		AllowedValues:   []string{"home", "full"},
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyWebRTCEnabled,
		Label:           "WebRTC Enabled",
		Description:     "Enable WebRTC remote streaming when dependencies are available.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "true",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:          SettingKeyWebRTCSTUNURL,
		Label:        "WebRTC STUN URL",
		Description:  "STUN server URL used for NAT traversal.",
		Type:         AgentSettingTypeString,
		DefaultValue: "stun:stun.l.google.com:19302",
		HubManaged:   true,
	},
	{
		Key:          SettingKeyWebRTCTURNURL,
		Label:        "WebRTC TURN URL",
		Description:  "Optional TURN server URL used when direct ICE fails.",
		Type:         AgentSettingTypeString,
		DefaultValue: "",
		HubManaged:   true,
	},
	{
		Key:          SettingKeyWebRTCTURNUser,
		Label:        "WebRTC TURN Username",
		Description:  "Optional TURN username.",
		Type:         AgentSettingTypeString,
		DefaultValue: "",
		HubManaged:   true,
	},
	{
		Key:          SettingKeyWebRTCTURNPass,
		Label:        "WebRTC TURN Password",
		Description:  "Optional TURN password.",
		Type:         AgentSettingTypeString,
		DefaultValue: "",
		HubManaged:   true,
	},
	{
		Key:             SettingKeyWebRTCWaylandExperimentalEnabled,
		Label:           "Wayland WebRTC Experimental",
		Description:     "Enable the experimental Wayland real-desktop WebRTC backend on Linux.",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "false",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyWebRTCWaylandPipeWireNodeID,
		Label:           "Wayland PipeWire Node ID",
		Description:     "PipeWire node ID used by the experimental Wayland WebRTC backend.",
		Type:            AgentSettingTypeString,
		DefaultValue:    "",
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:             SettingKeyWebRTCWaylandInputBackend,
		Label:           "Wayland Input Backend",
		Description:     "Input injector used by the experimental Wayland WebRTC backend.",
		Type:            AgentSettingTypeEnum,
		DefaultValue:    "auto",
		AllowedValues:   []string{"auto", "none", "ydotool"},
		HubManaged:      true,
		RestartRequired: true,
	},
	{
		Key:          SettingKeyCaptureFPS,
		Label:        "Capture FPS",
		Description:  "Target capture frame rate for WebRTC sessions.",
		Type:         AgentSettingTypeInt,
		DefaultValue: "30",
		MinInt:       5,
		MaxInt:       120,
		HubManaged:   true,
	},
	{
		Key:             SettingKeyTLSSkipVerify,
		Label:           "TLS Skip Verify",
		Description:     "Skip TLS certificate verification (bootstrap use only).",
		Type:            AgentSettingTypeBool,
		DefaultValue:    "false",
		RestartRequired: true,
		LocalOnly:       true,
	},
	{
		Key:             SettingKeyTLSCAFile,
		Label:           "TLS CA File",
		Description:     "Path to CA certificate PEM used to verify the hub TLS certificate.",
		Type:            AgentSettingTypeString,
		DefaultValue:    "",
		RestartRequired: true,
		LocalOnly:       true,
	},
	{
		Key:          SettingKeyAllowRemoteOverrides,
		Label:        "Allow Remote Overrides",
		Description:  "Allow hub-side settings updates to be applied remotely.",
		Type:         AgentSettingTypeBool,
		DefaultValue: "true",
		LocalOnly:    true,
	},
	{
		Key:           SettingKeyLogLevel,
		Label:         "Log Level",
		Description:   "Agent log verbosity.",
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

func AgentSettingDefinitions() []AgentSettingDefinition {
	out := make([]AgentSettingDefinition, len(agentSettingDefinitions))
	copy(out, agentSettingDefinitions)
	return out
}

func AgentSettingDefinitionByKey(key string) (AgentSettingDefinition, bool) {
	item, ok := agentSettingDefinitionsByKey[strings.TrimSpace(strings.ToLower(key))]
	return item, ok
}

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

func DefaultAgentSettingValues() map[string]string {
	out := make(map[string]string, len(agentSettingDefinitions))
	for _, definition := range agentSettingDefinitions {
		out[definition.Key] = definition.DefaultValue
	}
	return out
}

func NormalizeDiscoveryPortListValue(key, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}

	fields := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', ';', ' ', '\n', '\t':
			return true
		default:
			return false
		}
	})

	if len(fields) == 0 {
		return "", nil
	}

	seen := make(map[int]struct{}, len(fields))
	ports := make([]int, 0, len(fields))
	for _, field := range fields {
		token := strings.TrimSpace(field)
		if token == "" {
			continue
		}
		port, err := strconv.Atoi(token)
		if err != nil || port <= 0 || port > 65535 {
			return "", fmt.Errorf("%s must contain only TCP ports between 1 and 65535", key)
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}

	sort.Ints(ports)
	items := make([]string, 0, len(ports))
	for _, port := range ports {
		items = append(items, strconv.Itoa(port))
	}
	return strings.Join(items, ","), nil
}

func NormalizeDiscoveryCIDRListValue(key, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}

	fields := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', ';', ' ', '\n', '\t':
			return true
		default:
			return false
		}
	})
	if len(fields) == 0 {
		return "", nil
	}

	seen := make(map[string]struct{}, len(fields))
	cidrs := make([]string, 0, len(fields))
	for _, field := range fields {
		token := strings.TrimSpace(field)
		if token == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(token)
		if err != nil || !prefix.IsValid() {
			return "", fmt.Errorf("%s must contain valid CIDR values", key)
		}

		addr := prefix.Addr()
		if !addr.Is4() && !addr.Is6() {
			return "", fmt.Errorf("%s only supports IPv4 or IPv6 CIDR values", key)
		}
		if !isPrivateOrLocalCIDR(prefix) {
			return "", fmt.Errorf("%s only allows private/local CIDR ranges", key)
		}

		normalized := prefix.Masked().String()
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		cidrs = append(cidrs, normalized)
	}

	sort.Strings(cidrs)
	return strings.Join(cidrs, ","), nil
}

func isPrivateOrLocalCIDR(prefix netip.Prefix) bool {
	addr := prefix.Addr()
	if !addr.IsValid() {
		return false
	}
	if addr.IsLoopback() || addr.IsPrivate() {
		return true
	}
	if addr.Is4() {
		ip := net.ParseIP(addr.String())
		return ip != nil && ip.IsLinkLocalUnicast()
	}
	return addr.Is6() && addr.IsLinkLocalUnicast()
}

func normalizeDockerEndpointValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s cannot be empty", SettingKeyDockerEndpoint)
	}

	if strings.HasPrefix(value, "/") {
		return value, nil
	}
	if path, ok := dockerpkg.TrimDockerUnixScheme(value); ok {
		if path == "" || !strings.HasPrefix(path, "/") {
			return "", fmt.Errorf("%s unix path must be absolute", SettingKeyDockerEndpoint)
		}
		return "unix://" + path, nil
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%s must be an absolute unix path, unix:// path, or http(s) URL", SettingKeyDockerEndpoint)
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%s URL scheme must be http or https", SettingKeyDockerEndpoint)
	}

	validated, err := securityruntime.ValidateOutboundURL(value)
	if err != nil {
		return "", fmt.Errorf("%s is not allowed by outbound policy: %w", SettingKeyDockerEndpoint, err)
	}
	validated.RawQuery = ""
	validated.Fragment = ""
	return strings.TrimRight(validated.String(), "/"), nil
}

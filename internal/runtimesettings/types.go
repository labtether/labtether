package runtimesettings

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ValueType string

const (
	ValueTypeString   ValueType = "string"
	ValueTypeBool     ValueType = "bool"
	ValueTypeInt      ValueType = "int"
	ValueTypeDuration ValueType = "duration"
	ValueTypeEnum     ValueType = "enum"
	ValueTypeURL      ValueType = "url"
)

type Source string

const (
	SourceUI      Source = "ui"
	SourceDocker  Source = "docker"
	SourceDefault Source = "default"
)

type Definition struct {
	Key           string
	Label         string
	Description   string
	Scope         string
	Type          ValueType
	EnvVar        string
	DefaultValue  string
	MinInt        int
	MaxInt        int
	AllowedValues []string
	AllowEmpty    bool
}

const (
	KeyConsolePollIntervalSeconds   = "console.poll_interval_seconds"
	KeyConsoleDefaultTelemetry      = "console.default_telemetry_window"
	KeyConsoleDefaultLogWindow      = "console.default_log_window"
	KeyConsoleLogQueryLimit         = "console.log_query_limit"
	KeyConsoleDefaultActorID        = "console.default_actor_id"
	KeyConsoleDefaultActionDryRun   = "console.default_action_dry_run"
	KeyConsoleDefaultUpdateDryRun   = "console.default_update_dry_run"
	KeyRoutingAPIBaseURL            = "routing.api_base_url"
	KeyRoutingAgentBaseURL          = "routing.agent_base_url"
	KeyWorkerQueueMaxDeliveries     = "worker.queue_max_deliveries"
	KeyWorkerRetentionApply         = "worker.retention_apply_interval"
	KeyPolicyStructuredEnabled      = "policy.structured_enabled"
	KeyPolicyInteractiveEnabled     = "policy.interactive_enabled"
	KeyPolicyConnectorEnabled       = "policy.connector_enabled"
	KeySecurityOutboundAllowPrivate = "security.outbound_allow_private"
	KeyRemoteAccessMode             = "remote_access.mode"
	KeyRemoteAccessTailscaleTarget  = "remote_access.tailscale_serve_target"

	KeyServicesDiscoveryDefaultDockerEnabled            = "services.discovery_default_docker_enabled"
	KeyServicesDiscoveryDefaultProxyEnabled             = "services.discovery_default_proxy_enabled"
	KeyServicesDiscoveryDefaultProxyTraefikEnabled      = "services.discovery_default_proxy_traefik_enabled"
	KeyServicesDiscoveryDefaultProxyCaddyEnabled        = "services.discovery_default_proxy_caddy_enabled"
	KeyServicesDiscoveryDefaultProxyNPMEnabled          = "services.discovery_default_proxy_npm_enabled"
	KeyServicesDiscoveryDefaultPortScanEnabled          = "services.discovery_default_port_scan_enabled"
	KeyServicesDiscoveryDefaultPortScanIncludeListening = "services.discovery_default_port_scan_include_listening"
	KeyServicesDiscoveryDefaultPortScanPorts            = "services.discovery_default_port_scan_ports"
	KeyServicesDiscoveryDefaultLANScanEnabled           = "services.discovery_default_lan_scan_enabled"
	KeyServicesDiscoveryDefaultLANScanCIDRs             = "services.discovery_default_lan_scan_cidrs"
	KeyServicesDiscoveryDefaultLANScanPorts             = "services.discovery_default_lan_scan_ports"
	KeyServicesDiscoveryDefaultLANScanMaxHosts          = "services.discovery_default_lan_scan_max_hosts"

	KeyPrometheusScrapeEnabled       = "prometheus.scrape_enabled"
	KeyPrometheusRemoteWriteEnabled  = "prometheus.remote_write_enabled"
	KeyPrometheusRemoteWriteURL      = "prometheus.remote_write_url"
	KeyPrometheusRemoteWriteUsername = "prometheus.remote_write_username"
	KeyPrometheusRemoteWritePassword = "prometheus.remote_write_password"
	KeyPrometheusRemoteWriteInterval = "prometheus.remote_write_interval"
	KeyProcessMetricsEnabled         = "prometheus.process_metrics_enabled"
	KeyProcessMetricsTopN            = "prometheus.process_metrics_top_n"
)

var definitions = []Definition{
	{
		Key:          KeyConsolePollIntervalSeconds,
		Label:        "Status Poll Interval",
		Description:  "Dashboard status refresh interval in seconds.",
		Scope:        "console",
		Type:         ValueTypeInt,
		EnvVar:       "LABTETHER_POLL_INTERVAL_SECONDS",
		DefaultValue: "5",
		MinInt:       2,
		MaxInt:       120,
	},
	{
		Key:           KeyConsoleDefaultTelemetry,
		Label:         "Default Telemetry Window",
		Description:   "Default telemetry range selection.",
		Scope:         "console",
		Type:          ValueTypeEnum,
		EnvVar:        "LABTETHER_DEFAULT_TELEMETRY_WINDOW",
		DefaultValue:  "1h",
		AllowedValues: []string{"15m", "1h", "6h", "24h"},
	},
	{
		Key:           KeyConsoleDefaultLogWindow,
		Label:         "Default Logs Window",
		Description:   "Default log query range selection.",
		Scope:         "console",
		Type:          ValueTypeEnum,
		EnvVar:        "LABTETHER_DEFAULT_LOG_WINDOW",
		DefaultValue:  "1h",
		AllowedValues: []string{"15m", "1h", "6h", "24h"},
	},
	{
		Key:          KeyConsoleLogQueryLimit,
		Label:        "Log Query Limit",
		Description:  "Maximum events requested per log query.",
		Scope:        "console",
		Type:         ValueTypeInt,
		EnvVar:       "LABTETHER_LOG_QUERY_LIMIT",
		DefaultValue: "120",
		MinInt:       20,
		MaxInt:       500,
	},
	{
		Key:          KeyConsoleDefaultActorID,
		Label:        "Default Actor ID",
		Description:  "Default actor identity used for command and action requests.",
		Scope:        "console",
		Type:         ValueTypeString,
		EnvVar:       "LABTETHER_DEFAULT_ACTOR_ID",
		DefaultValue: "owner",
	},
	{
		Key:          KeyConsoleDefaultActionDryRun,
		Label:        "Default Action Dry Run",
		Description:  "Default dry-run mode for connector actions.",
		Scope:        "console",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_DEFAULT_ACTION_DRY_RUN",
		DefaultValue: "true",
	},
	{
		Key:          KeyConsoleDefaultUpdateDryRun,
		Label:        "Default Update Dry Run",
		Description:  "Default dry-run mode for update plan execution.",
		Scope:        "console",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_DEFAULT_UPDATE_DRY_RUN",
		DefaultValue: "true",
	},
	{
		Key:          KeyRoutingAPIBaseURL,
		Label:        "API Base URL",
		Description:  "Base URL used by the web proxy for API calls.",
		Scope:        "routing",
		Type:         ValueTypeURL,
		EnvVar:       "LABTETHER_API_BASE_URL",
		DefaultValue: "http://localhost:8080",
	},
	{
		Key:          KeyRoutingAgentBaseURL,
		Label:        "Agent Base URL",
		Description:  "Base URL used by the web proxy for Linux agent probes.",
		Scope:        "routing",
		Type:         ValueTypeURL,
		EnvVar:       "LABTETHER_AGENT_BASE_URL",
		DefaultValue: "http://localhost:8090",
	},
	{
		Key:          KeyWorkerQueueMaxDeliveries,
		Label:        "Queue Max Deliveries",
		Description:  "Maximum deliveries before moving a message to dead-letter.",
		Scope:        "worker",
		Type:         ValueTypeInt,
		EnvVar:       "QUEUE_MAX_DELIVERIES",
		DefaultValue: "5",
		MinInt:       1,
		MaxInt:       50,
	},
	{
		Key:          KeyWorkerRetentionApply,
		Label:        "Retention Apply Interval",
		Description:  "Frequency for worker retention prune cycle.",
		Scope:        "worker",
		Type:         ValueTypeDuration,
		EnvVar:       "RETENTION_APPLY_INTERVAL",
		DefaultValue: "5m",
	},
	{
		Key:          KeyPolicyStructuredEnabled,
		Label:        "Structured Commands Enabled",
		Description:  "Allow structured command mode in policy checks.",
		Scope:        "policy",
		Type:         ValueTypeBool,
		EnvVar:       "STRUCTURED_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyPolicyInteractiveEnabled,
		Label:        "Interactive Commands Enabled",
		Description:  "Allow interactive command mode in policy checks.",
		Scope:        "policy",
		Type:         ValueTypeBool,
		EnvVar:       "INTERACTIVE_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyPolicyConnectorEnabled,
		Label:        "Connector Actions Enabled",
		Description:  "Allow connector action execution in policy checks.",
		Scope:        "policy",
		Type:         ValueTypeBool,
		EnvVar:       "CONNECTOR_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:           KeySecurityOutboundAllowPrivate,
		Label:         "Private Outbound HTTPS/WSS Targets",
		Description:   "auto allows secure private-network connector targets by default; true always allows private outbound hosts, and false restores strict deny-private behavior.",
		Scope:         "security",
		Type:          ValueTypeEnum,
		EnvVar:        "LABTETHER_OUTBOUND_ALLOW_PRIVATE",
		DefaultValue:  "auto",
		AllowedValues: []string{"auto", "true", "false"},
	},
	{
		Key:           KeyRemoteAccessMode,
		Label:         "Remote Access Mode",
		Description:   "Preferred operator-facing remote access mode. serve keeps Tailscale HTTPS recommended, manual assumes you will manage access yourself, and off suppresses the recommendation for now.",
		Scope:         "remote_access",
		Type:          ValueTypeEnum,
		EnvVar:        "LABTETHER_REMOTE_ACCESS_MODE",
		DefaultValue:  "serve",
		AllowedValues: []string{"serve", "manual", "off"},
	},
	{
		Key:          KeyRemoteAccessTailscaleTarget,
		Label:        "Tailscale Serve Upstream URL",
		Description:  "Optional host-local upstream URL for tailscale serve. Leave blank to let LabTether suggest the local hub URL automatically.",
		Scope:        "remote_access",
		Type:         ValueTypeURL,
		EnvVar:       "LABTETHER_TAILSCALE_SERVE_TARGET",
		DefaultValue: "",
		AllowEmpty:   true,
	},
	{
		Key:          KeyServicesDiscoveryDefaultDockerEnabled,
		Label:        "Default Agent Docker Discovery",
		Description:  "Default agent setting for Docker-backed service discovery.",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_DOCKER_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyServicesDiscoveryDefaultProxyEnabled,
		Label:        "Default Agent Proxy API Discovery",
		Description:  "Default agent setting for reverse-proxy API discovery.",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PROXY_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyServicesDiscoveryDefaultProxyTraefikEnabled,
		Label:        "Default Agent Traefik API Discovery",
		Description:  "Default agent setting for Traefik API route discovery.",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PROXY_TRAEFIK_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyServicesDiscoveryDefaultProxyCaddyEnabled,
		Label:        "Default Agent Caddy API Discovery",
		Description:  "Default agent setting for Caddy admin API route discovery.",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PROXY_CADDY_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyServicesDiscoveryDefaultProxyNPMEnabled,
		Label:        "Default Agent Nginx Proxy Manager Discovery",
		Description:  "Default agent setting for Nginx Proxy Manager API route discovery.",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PROXY_NPM_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyServicesDiscoveryDefaultPortScanEnabled,
		Label:        "Default Agent Local Port Scan",
		Description:  "Default agent setting for local host port scanning.",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PORT_SCAN_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyServicesDiscoveryDefaultPortScanIncludeListening,
		Label:        "Default Agent Scan Listening Ports",
		Description:  "Default agent setting for including listening sockets in local port scans.",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PORT_SCAN_INCLUDE_LISTENING",
		DefaultValue: "true",
	},
	{
		Key:          KeyServicesDiscoveryDefaultPortScanPorts,
		Label:        "Default Agent Local Scan Ports",
		Description:  "Optional default local scan port list for agents.",
		Scope:        "services",
		Type:         ValueTypeString,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PORT_SCAN_PORTS",
		DefaultValue: "",
		AllowEmpty:   true,
	},
	{
		Key:          KeyServicesDiscoveryDefaultLANScanEnabled,
		Label:        "Default Agent LAN Scan",
		Description:  "Default agent setting for LAN CIDR service scanning.",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_LAN_SCAN_ENABLED",
		DefaultValue: "false",
	},
	{
		Key:          KeyServicesDiscoveryDefaultLANScanCIDRs,
		Label:        "Default Agent LAN CIDRs",
		Description:  "Optional default CIDR list for agent LAN scanning.",
		Scope:        "services",
		Type:         ValueTypeString,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_LAN_SCAN_CIDRS",
		DefaultValue: "",
		AllowEmpty:   true,
	},
	{
		Key:          KeyServicesDiscoveryDefaultLANScanPorts,
		Label:        "Default Agent LAN Scan Ports",
		Description:  "Optional default port list for agent LAN scanning.",
		Scope:        "services",
		Type:         ValueTypeString,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_LAN_SCAN_PORTS",
		DefaultValue: "",
		AllowEmpty:   true,
	},
	{
		Key:          KeyServicesDiscoveryDefaultLANScanMaxHosts,
		Label:        "Default Agent LAN Scan Host Cap",
		Description:  "Default maximum LAN hosts probed per agent scan cycle.",
		Scope:        "services",
		Type:         ValueTypeInt,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_LAN_SCAN_MAX_HOSTS",
		DefaultValue: "64",
		MinInt:       1,
		MaxInt:       1024,
	},
	{
		Key:          KeyPrometheusScrapeEnabled,
		Label:        "Prometheus Scrape Enabled",
		Description:  "Expose the /metrics endpoint for Prometheus scraping.",
		Scope:        "prometheus",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_PROMETHEUS_SCRAPE_ENABLED",
		DefaultValue: "false",
	},
	{
		Key:          KeyPrometheusRemoteWriteEnabled,
		Label:        "Prometheus Remote Write Enabled",
		Description:  "Push metrics to a Prometheus-compatible remote_write endpoint.",
		Scope:        "prometheus",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_PROMETHEUS_REMOTE_WRITE_ENABLED",
		DefaultValue: "false",
	},
	{
		Key:          KeyPrometheusRemoteWriteURL,
		Label:        "Remote Write URL",
		Description:  "Target URL for Prometheus remote_write pushes (e.g. https://prometheus.example.com/api/v1/write).",
		Scope:        "prometheus",
		Type:         ValueTypeURL,
		EnvVar:       "LABTETHER_PROMETHEUS_REMOTE_WRITE_URL",
		DefaultValue: "",
		AllowEmpty:   true,
	},
	{
		Key:          KeyPrometheusRemoteWriteUsername,
		Label:        "Remote Write Username",
		Description:  "Optional HTTP Basic Auth username for remote_write.",
		Scope:        "prometheus",
		Type:         ValueTypeString,
		EnvVar:       "LABTETHER_PROMETHEUS_REMOTE_WRITE_USERNAME",
		DefaultValue: "",
		AllowEmpty:   true,
	},
	{
		Key:          KeyPrometheusRemoteWritePassword,
		Label:        "Remote Write Password",
		Description:  "Optional HTTP Basic Auth password for remote_write. Stored encrypted when a secrets manager is available.",
		Scope:        "prometheus",
		Type:         ValueTypeString,
		EnvVar:       "LABTETHER_PROMETHEUS_REMOTE_WRITE_PASSWORD",
		DefaultValue: "",
		AllowEmpty:   true,
	},
	{
		Key:          KeyPrometheusRemoteWriteInterval,
		Label:        "Remote Write Interval",
		Description:  "How often to push metrics to the remote_write endpoint.",
		Scope:        "prometheus",
		Type:         ValueTypeDuration,
		EnvVar:       "LABTETHER_PROMETHEUS_REMOTE_WRITE_INTERVAL",
		DefaultValue: "30s",
	},
	{
		Key:          KeyProcessMetricsEnabled,
		Label:        "Process Metrics Enabled",
		Description:  "Collect per-process CPU and memory metrics from managed assets.",
		Scope:        "prometheus",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_PROCESS_METRICS_ENABLED",
		DefaultValue: "false",
	},
	{
		Key:          KeyProcessMetricsTopN,
		Label:        "Process Metrics Top N",
		Description:  "Maximum number of top processes to track per asset.",
		Scope:        "prometheus",
		Type:         ValueTypeInt,
		EnvVar:       "LABTETHER_PROCESS_METRICS_TOP_N",
		DefaultValue: "20",
		MinInt:       1,
		MaxInt:       200,
	},
}

var definitionsByKey = buildDefinitionsByKey(definitions)

func buildDefinitionsByKey(items []Definition) map[string]Definition {
	out := make(map[string]Definition, len(items))
	for _, item := range items {
		out[item.Key] = item
	}
	return out
}

func Definitions() []Definition {
	out := make([]Definition, len(definitions))
	copy(out, definitions)
	return out
}

func DefinitionByKey(key string) (Definition, bool) {
	item, ok := definitionsByKey[key]
	return item, ok
}

func NormalizeValue(def Definition, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	switch def.Type {
	case ValueTypeString:
		if value == "" && !def.AllowEmpty {
			return "", fmt.Errorf("%s cannot be empty", def.Key)
		}
		return value, nil
	case ValueTypeBool:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return "", fmt.Errorf("%s must be true or false", def.Key)
		}
		return strconv.FormatBool(parsed), nil
	case ValueTypeInt:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return "", fmt.Errorf("%s must be a number", def.Key)
		}
		if def.MinInt > 0 && parsed < def.MinInt {
			return "", fmt.Errorf("%s must be >= %d", def.Key, def.MinInt)
		}
		if def.MaxInt > 0 && parsed > def.MaxInt {
			return "", fmt.Errorf("%s must be <= %d", def.Key, def.MaxInt)
		}
		return strconv.Itoa(parsed), nil
	case ValueTypeDuration:
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return "", fmt.Errorf("%s must be a positive duration", def.Key)
		}
		return parsed.String(), nil
	case ValueTypeEnum:
		for _, allowed := range def.AllowedValues {
			if strings.EqualFold(value, allowed) {
				return allowed, nil
			}
		}
		return "", fmt.Errorf("%s must be one of: %s", def.Key, strings.Join(def.AllowedValues, ", "))
	case ValueTypeURL:
		if value == "" && def.AllowEmpty {
			return "", nil
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return "", fmt.Errorf("%s must be an absolute URL", def.Key)
		}
		return strings.TrimRight(parsed.String(), "/"), nil
	default:
		return "", fmt.Errorf("unsupported runtime setting type for %s", def.Key)
	}
}

func ResolveEnvValue(def Definition, lookup func(string) string) string {
	if lookup == nil || strings.TrimSpace(def.EnvVar) == "" {
		return ""
	}
	raw := strings.TrimSpace(lookup(def.EnvVar))
	if raw == "" {
		return ""
	}
	normalized, err := NormalizeValue(def, raw)
	if err != nil {
		return ""
	}
	return normalized
}

func EffectiveValue(def Definition, envValue, overrideValue string) (string, Source) {
	if strings.TrimSpace(overrideValue) != "" {
		return overrideValue, SourceUI
	}
	if strings.TrimSpace(envValue) != "" {
		return envValue, SourceDocker
	}
	return def.DefaultValue, SourceDefault
}

package runtimesettings

import (
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
	Key                string
	Label              string
	Description        string
	Scope              string
	Type               ValueType
	EnvVar             string
	DefaultValue       string
	Sensitive          bool
	MinInt             int
	MaxInt             int
	MinDuration        time.Duration
	MaxDuration        time.Duration
	MaxBytes           int
	PreserveWhitespace bool
	AllowedValues      []string
	AllowEmpty         bool
}

const (
	KeyConsolePollIntervalSeconds     = "console.poll_interval_seconds"
	KeyConsoleDefaultTelemetry        = "console.default_telemetry_window"
	KeyConsoleDefaultLogWindow        = "console.default_log_window"
	KeyConsoleLogQueryLimit           = "console.log_query_limit"
	KeyConsoleDefaultActorID          = "console.default_actor_id"
	KeyConsoleDefaultActionDryRun     = "console.default_action_dry_run"
	KeyConsoleDefaultUpdateDryRun     = "console.default_update_dry_run"
	KeyRoutingAPIBaseURL              = "routing.api_base_url"
	KeyRoutingAgentBaseURL            = "routing.agent_base_url"
	KeyWorkerQueueMaxDeliveries       = "worker.queue_max_deliveries"
	KeyWorkerRetentionApply           = "worker.retention_apply_interval"
	KeyPolicyStructuredEnabled        = "policy.structured_enabled"
	KeyPolicyInteractiveEnabled       = "policy.interactive_enabled"
	KeyPolicyConnectorEnabled         = "policy.connector_enabled"
	KeySecurityOutboundAllowPrivate   = "security.outbound_allow_private"
	KeySecurityOutboundAllowLinkLocal = "security.outbound_allow_link_local"
	KeyRemoteAccessMode               = "remote_access.mode"
	KeyRemoteAccessTailscaleTarget    = "remote_access.tailscale_serve_target"

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
		Label:        "Status refresh",
		Description:  "Seconds between dashboard updates.",
		Scope:        "console",
		Type:         ValueTypeInt,
		EnvVar:       "LABTETHER_POLL_INTERVAL_SECONDS",
		DefaultValue: "5",
		MinInt:       2,
		MaxInt:       120,
	},
	{
		Key:           KeyConsoleDefaultTelemetry,
		Label:         "Chart range",
		Description:   "The range charts open with.",
		Scope:         "console",
		Type:          ValueTypeEnum,
		EnvVar:        "LABTETHER_DEFAULT_TELEMETRY_WINDOW",
		DefaultValue:  "1h",
		AllowedValues: []string{"15m", "1h", "6h", "24h"},
	},
	{
		Key:           KeyConsoleDefaultLogWindow,
		Label:         "Logs range",
		Description:   "The range Logs opens with.",
		Scope:         "console",
		Type:          ValueTypeEnum,
		EnvVar:        "LABTETHER_DEFAULT_LOG_WINDOW",
		DefaultValue:  "1h",
		AllowedValues: []string{"15m", "1h", "6h", "24h"},
	},
	{
		Key:          KeyConsoleLogQueryLimit,
		Label:        "Log lines per query",
		Description:  "",
		Scope:        "console",
		Type:         ValueTypeInt,
		EnvVar:       "LABTETHER_LOG_QUERY_LIMIT",
		DefaultValue: "120",
		MinInt:       20,
		MaxInt:       500,
	},
	{
		Key:          KeyConsoleDefaultActorID,
		Label:        "Default user ID",
		Description:  "Recorded as the person for commands and actions that name no user.",
		Scope:        "console",
		Type:         ValueTypeString,
		EnvVar:       "LABTETHER_DEFAULT_ACTOR_ID",
		DefaultValue: "owner",
	},
	{
		Key:          KeyConsoleDefaultActionDryRun,
		Label:        "Dry run for integration actions",
		Description:  "",
		Scope:        "console",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_DEFAULT_ACTION_DRY_RUN",
		DefaultValue: "true",
	},
	{
		Key:          KeyConsoleDefaultUpdateDryRun,
		Label:        "Dry run for updates",
		Description:  "",
		Scope:        "console",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_DEFAULT_UPDATE_DRY_RUN",
		DefaultValue: "true",
	},
	{
		Key:          KeyRoutingAPIBaseURL,
		Label:        "API address",
		Description:  "Where the console server reaches the hub API.",
		Scope:        "routing",
		Type:         ValueTypeURL,
		EnvVar:       "LABTETHER_API_BASE_URL",
		DefaultValue: "http://localhost:8080",
	},
	{
		Key:          KeyRoutingAgentBaseURL,
		Label:        "Agent address",
		Description:  "Where the console server reaches a Linux agent.",
		Scope:        "routing",
		Type:         ValueTypeURL,
		EnvVar:       "LABTETHER_AGENT_BASE_URL",
		DefaultValue: "http://localhost:8090",
	},
	{
		Key:          KeyWorkerQueueMaxDeliveries,
		Label:        "Job attempts",
		Description:  "Tries before a failing job is set aside.",
		Scope:        "worker",
		Type:         ValueTypeInt,
		EnvVar:       "QUEUE_MAX_DELIVERIES",
		DefaultValue: "5",
		MinInt:       1,
		MaxInt:       50,
	},
	{
		Key:          KeyWorkerRetentionApply,
		Label:        "Retention interval",
		Description:  "How often the hub deletes data older than its retention.",
		Scope:        "worker",
		Type:         ValueTypeDuration,
		EnvVar:       "RETENTION_APPLY_INTERVAL",
		DefaultValue: "5m",
	},
	{
		Key:          KeyPolicyStructuredEnabled,
		Label:        "Commands",
		Description:  "Allow commands from actions, schedules and the API.",
		Scope:        "policy",
		Type:         ValueTypeBool,
		EnvVar:       "STRUCTURED_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyPolicyInteractiveEnabled,
		Label:        "Terminal sessions",
		Description:  "",
		Scope:        "policy",
		Type:         ValueTypeBool,
		EnvVar:       "INTERACTIVE_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyPolicyConnectorEnabled,
		Label:        "Integration actions",
		Description:  "",
		Scope:        "policy",
		Type:         ValueTypeBool,
		EnvVar:       "CONNECTOR_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:           KeySecurityOutboundAllowPrivate,
		Label:         "Private network targets",
		Description:   "auto: allow HTTPS and WSS integrations on private addresses. true: allow every private address. false: block private addresses.",
		Scope:         "security",
		Type:          ValueTypeEnum,
		EnvVar:        "LABTETHER_OUTBOUND_ALLOW_PRIVATE",
		DefaultValue:  "auto",
		AllowedValues: []string{"auto", "true", "false"},
	},
	{
		Key:          KeySecurityOutboundAllowLinkLocal,
		Label:        "Link-local targets",
		Description:  "Allow connections to link-local addresses (169.254.0.0/16 and fe80::/10).",
		Scope:        "security",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_OUTBOUND_ALLOW_LINK_LOCAL",
		DefaultValue: "false",
	},
	{
		Key:           KeyRemoteAccessMode,
		Label:         "Remote access",
		Description:   "serve: suggest Tailscale Serve for HTTPS. manual: you set up access yourself. off: no suggestion.",
		Scope:         "remote_access",
		Type:          ValueTypeEnum,
		EnvVar:        "LABTETHER_REMOTE_ACCESS_MODE",
		DefaultValue:  "serve",
		AllowedValues: []string{"serve", "manual", "off"},
	},
	{
		Key:          KeyRemoteAccessTailscaleTarget,
		Label:        "Tailscale Serve target",
		Description:  "The local address Tailscale Serve forwards to. Empty uses the hub's own address.",
		Scope:        "remote_access",
		Type:         ValueTypeURL,
		EnvVar:       "LABTETHER_TAILSCALE_SERVE_TARGET",
		DefaultValue: "",
		AllowEmpty:   true,
	},
	{
		Key:          KeyServicesDiscoveryDefaultDockerEnabled,
		Label:        "Docker containers",
		Description:  "Find services from container ports and labels.",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_DOCKER_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyServicesDiscoveryDefaultProxyEnabled,
		Label:        "Reverse proxies",
		Description:  "Read routed services from the proxy APIs below.",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PROXY_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyServicesDiscoveryDefaultProxyTraefikEnabled,
		Label:        "Traefik routes",
		Description:  "",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PROXY_TRAEFIK_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyServicesDiscoveryDefaultProxyCaddyEnabled,
		Label:        "Caddy routes",
		Description:  "From the Caddy admin API.",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PROXY_CADDY_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyServicesDiscoveryDefaultProxyNPMEnabled,
		Label:        "Nginx Proxy Manager routes",
		Description:  "",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PROXY_NPM_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyServicesDiscoveryDefaultPortScanEnabled,
		Label:        "Local port scan",
		Description:  "Probe the device's own ports for web services.",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PORT_SCAN_ENABLED",
		DefaultValue: "true",
	},
	{
		Key:          KeyServicesDiscoveryDefaultPortScanIncludeListening,
		Label:        "Add listening ports",
		Description:  "Also scan every TCP port the device listens on.",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PORT_SCAN_INCLUDE_LISTENING",
		DefaultValue: "true",
	},
	{
		Key:          KeyServicesDiscoveryDefaultPortScanPorts,
		Label:        "Local scan ports",
		Description:  "Comma separated. Empty uses the built-in list.",
		Scope:        "services",
		Type:         ValueTypeString,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PORT_SCAN_PORTS",
		DefaultValue: "",
		AllowEmpty:   true,
	},
	{
		Key:          KeyServicesDiscoveryDefaultLANScanEnabled,
		Label:        "LAN scan",
		Description:  "Probe the LAN ranges for web services.",
		Scope:        "services",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_LAN_SCAN_ENABLED",
		DefaultValue: "false",
	},
	{
		Key:          KeyServicesDiscoveryDefaultLANScanCIDRs,
		Label:        "LAN ranges",
		Description:  "Private CIDRs, comma separated.",
		Scope:        "services",
		Type:         ValueTypeString,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_LAN_SCAN_CIDRS",
		DefaultValue: "",
		AllowEmpty:   true,
	},
	{
		Key:          KeyServicesDiscoveryDefaultLANScanPorts,
		Label:        "LAN scan ports",
		Description:  "Comma separated. Empty uses the built-in list.",
		Scope:        "services",
		Type:         ValueTypeString,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_LAN_SCAN_PORTS",
		DefaultValue: "",
		AllowEmpty:   true,
	},
	{
		Key:          KeyServicesDiscoveryDefaultLANScanMaxHosts,
		Label:        "Hosts per LAN scan",
		Description:  "",
		Scope:        "services",
		Type:         ValueTypeInt,
		EnvVar:       "LABTETHER_SERVICES_DISCOVERY_DEFAULT_LAN_SCAN_MAX_HOSTS",
		DefaultValue: "64",
		MinInt:       1,
		MaxInt:       1024,
	},
	{
		Key:          KeyPrometheusScrapeEnabled,
		Label:        "Prometheus scraping",
		Description:  "Serve metrics at /metrics.",
		Scope:        "prometheus",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_PROMETHEUS_SCRAPE_ENABLED",
		DefaultValue: "false",
	},
	{
		Key:          KeyPrometheusRemoteWriteEnabled,
		Label:        "Prometheus remote write",
		Description:  "Push metrics to a remote_write endpoint.",
		Scope:        "prometheus",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_PROMETHEUS_REMOTE_WRITE_ENABLED",
		DefaultValue: "false",
	},
	{
		Key:          KeyPrometheusRemoteWriteURL,
		Label:        "Remote write URL",
		Description:  "For example https://prometheus.example.com/api/v1/write",
		Scope:        "prometheus",
		Type:         ValueTypeURL,
		EnvVar:       "LABTETHER_PROMETHEUS_REMOTE_WRITE_URL",
		DefaultValue: "",
		MaxBytes:     4096,
		AllowEmpty:   true,
	},
	{
		Key:          KeyPrometheusRemoteWriteUsername,
		Label:        "Remote write username",
		Description:  "For HTTP basic auth.",
		Scope:        "prometheus",
		Type:         ValueTypeString,
		EnvVar:       "LABTETHER_PROMETHEUS_REMOTE_WRITE_USERNAME",
		DefaultValue: "",
		MaxBytes:     512,
		AllowEmpty:   true,
	},
	{
		Key:                KeyPrometheusRemoteWritePassword,
		Label:              "Remote write password",
		Description:        "For HTTP basic auth. Stored encrypted when a secrets manager is available.",
		Scope:              "prometheus",
		Type:               ValueTypeString,
		EnvVar:             "LABTETHER_PROMETHEUS_REMOTE_WRITE_PASSWORD",
		DefaultValue:       "",
		Sensitive:          true,
		MaxBytes:           4096,
		PreserveWhitespace: true,
		AllowEmpty:         true,
	},
	{
		Key:          KeyPrometheusRemoteWriteInterval,
		Label:        "Remote write interval",
		Description:  "",
		Scope:        "prometheus",
		Type:         ValueTypeDuration,
		EnvVar:       "LABTETHER_PROMETHEUS_REMOTE_WRITE_INTERVAL",
		DefaultValue: "30s",
		MinDuration:  10 * time.Second,
		MaxDuration:  time.Hour,
	},
	{
		Key:          KeyProcessMetricsEnabled,
		Label:        "Process metrics",
		Description:  "Collect CPU and memory for each process on devices.",
		Scope:        "prometheus",
		Type:         ValueTypeBool,
		EnvVar:       "LABTETHER_PROCESS_METRICS_ENABLED",
		DefaultValue: "false",
	},
	{
		Key:          KeyProcessMetricsTopN,
		Label:        "Processes per device",
		Description:  "The busiest processes kept for each device.",
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

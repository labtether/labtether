export type RuntimeSettingEntry = {
  key: string;
  label: string;
  description: string;
  scope: string;
  type: string;
  env_var: string;
  default_value: string;
  env_value?: string;
  override_value?: string;
  effective_value: string;
  source: "ui" | "docker" | "default";
  sensitive: boolean;
  configured: boolean;
  allowed_values?: string[];
  min_int?: number;
  max_int?: number;
};

export type RuntimeSettingsPayload = {
  settings: RuntimeSettingEntry[];
  overrides: Record<string, string>;
  error?: string;
};

export type RetentionSettingsPayload = {
  settings: {
    logs_window: string;
    metrics_window: string;
    audit_window: string;
    terminal_window: string;
    action_runs_window: string;
    update_runs_window: string;
  };
  presets: Array<{
    id: string;
    name: string;
    description: string;
    settings: {
      logs_window: string;
      metrics_window: string;
      audit_window: string;
      terminal_window: string;
      action_runs_window: string;
      update_runs_window: string;
    };
  }>;
  error?: string;
};

export const sourceLabels: Record<RuntimeSettingEntry["source"], string> = {
  ui: "UI",
  docker: "Docker",
  default: "Default",
};

export const runtimeSettingKeys = {
  pollIntervalSeconds: "console.poll_interval_seconds",
  defaultTelemetryWindow: "console.default_telemetry_window",
  defaultLogWindow: "console.default_log_window",
  logQueryLimit: "console.log_query_limit",
  defaultActorID: "console.default_actor_id",
  defaultActionDryRun: "console.default_action_dry_run",
  defaultUpdateDryRun: "console.default_update_dry_run",
  remoteAccessMode: "remote_access.mode",
  remoteAccessTailscaleServeTarget: "remote_access.tailscale_serve_target",
  servicesMergeMode: "services.merge_mode",
  servicesMergeConfidenceThreshold: "services.merge_confidence_threshold",
  servicesMergeDryRun: "services.merge_dry_run",
  servicesMergeAliasRules: "services.merge_alias_rules",
  servicesForceMergeRules: "services.force_merge_rules",
  servicesNeverMergeRules: "services.never_merge_rules",
  servicesDiscoveryDefaultDockerEnabled: "services.discovery_default_docker_enabled",
  servicesDiscoveryDefaultProxyEnabled: "services.discovery_default_proxy_enabled",
  servicesDiscoveryDefaultProxyTraefikEnabled: "services.discovery_default_proxy_traefik_enabled",
  servicesDiscoveryDefaultProxyCaddyEnabled: "services.discovery_default_proxy_caddy_enabled",
  servicesDiscoveryDefaultProxyNPMEnabled: "services.discovery_default_proxy_npm_enabled",
  servicesDiscoveryDefaultPortScanEnabled: "services.discovery_default_port_scan_enabled",
  servicesDiscoveryDefaultPortScanIncludeListening: "services.discovery_default_port_scan_include_listening",
  servicesDiscoveryDefaultPortScanPorts: "services.discovery_default_port_scan_ports",
  servicesDiscoveryDefaultLANScanEnabled: "services.discovery_default_lan_scan_enabled",
  servicesDiscoveryDefaultLANScanCIDRs: "services.discovery_default_lan_scan_cidrs",
  servicesDiscoveryDefaultLANScanPorts: "services.discovery_default_lan_scan_ports",
  servicesDiscoveryDefaultLANScanMaxHosts: "services.discovery_default_lan_scan_max_hosts",
} as const;

export const retentionFieldDefs: Array<{
  key: keyof RetentionSettingsPayload["settings"];
  label: string;
}> = [
  { key: "logs_window", label: "Keep Logs For" },
  { key: "metrics_window", label: "Keep Metrics For" },
  { key: "audit_window", label: "Keep Audit Events For" },
  { key: "terminal_window", label: "Keep Shell History For" },
  { key: "action_runs_window", label: "Keep Action Results For" },
  { key: "update_runs_window", label: "Keep Update Results For" },
];

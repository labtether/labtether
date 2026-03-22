export type Theme = "oled" | "dark" | "light";
export type Density = "minimal" | "diagnostic";

export type EndpointStatus = {
  name: string;
  url: string;
  ok: boolean;
  status: "up" | "down";
  code?: number;
  latencyMs: number;
  error?: string;
};

export type Session = {
  id: string;
  target: string;
  mode: string;
  status: string;
  created_at: string;
  last_action_at: string;
};

export type Command = {
  id: string;
  session_id: string;
  body: string;
  status: string;
  output?: string;
  updated_at: string;
};

export type AuditEvent = {
  id: string;
  type: string;
  decision?: string;
  target?: string;
  timestamp: string;
};

export type LogEvent = {
  id: string;
  asset_id?: string;
  source: string;
  level: string;
  message: string;
  timestamp: string;
  fields?: Record<string, string>;
};

export type LogSource = {
  source: string;
  count: number;
  last_seen_at: string;
};

export type ConnectorDescriptor = {
  id: string;
  display_name: string;
};

export type ConnectorActionParameter = {
  key: string;
  label: string;
  required: boolean;
  description?: string;
};

export type ConnectorActionDescriptor = {
  id: string;
  name: string;
  description?: string;
  requires_target: boolean;
  supports_dry_run: boolean;
  parameters?: ConnectorActionParameter[];
};

export type ActionRunStep = {
  id: string;
  name: string;
  status: string;
  output?: string;
  error?: string;
  created_at: string;
  updated_at: string;
};

export type ActionRun = {
  id: string;
  type: string;
  actor_id: string;
  target?: string;
  command?: string;
  connector_id?: string;
  action_id?: string;
  status: string;
  output?: string;
  error?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
  steps?: ActionRunStep[];
};

export type UpdatePlan = {
  id: string;
  name: string;
  description?: string;
  targets: string[];
  scopes: string[];
  default_dry_run: boolean;
  created_at: string;
  updated_at: string;
};

export type UpdateRunResult = {
  target: string;
  scope: string;
  status: string;
  summary: string;
};

export type UpdateRun = {
  id: string;
  plan_id: string;
  plan_name: string;
  actor_id: string;
  dry_run: boolean;
  status: string;
  summary?: string;
  error?: string;
  results?: UpdateRunResult[];
  created_at: string;
  updated_at: string;
  completed_at?: string;
};

export type DeadLetterEvent = {
  id: string;
  component: string;
  subject: string;
  deliveries: number;
  error: string;
  payload_b64?: string;
  created_at: string;
};

export type DeadLetterTopEntry = {
  key: string;
  count: number;
};

export type DeadLetterTrendPoint = {
  start: string;
  end: string;
  count: number;
};

export type DeadLetterAnalytics = {
  window: string;
  bucket: string;
  total: number;
  rate_per_hour: number;
  rate_per_day: number;
  trend: DeadLetterTrendPoint[];
  top_components: DeadLetterTopEntry[];
  top_subjects: DeadLetterTopEntry[];
  top_error_classes: DeadLetterTopEntry[];
};

export type AlertInstance = {
  id: string;
  rule_id: string;
  fingerprint: string;
  status: "pending" | "firing" | "acknowledged" | "resolved";
  severity: "critical" | "high" | "medium" | "low";
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  started_at: string;
  resolved_at?: string;
  last_fired_at: string;
  suppressed_by?: string;
  created_at: string;
  updated_at: string;
};

export type AlertRule = {
  id: string;
  name: string;
  description?: string;
  status: "active" | "paused";
  kind:
    | "metric_threshold"
    | "metric_deadman"
    | "heartbeat_stale"
    | "log_pattern"
    | "composite"
    | "synthetic_check";
  severity: "critical" | "high" | "medium" | "low";
  target_scope: "asset" | "group" | "global";
  cooldown_seconds: number;
  window_seconds: number;
  condition: Record<string, unknown>;
  labels?: Record<string, string>;
  targets?: Array<{ id: string; asset_id?: string; group_id?: string }>;
  created_at: string;
  updated_at: string;
  last_evaluated_at?: string;
};

export type AlertRuleTemplate = {
  id: string;
  name: string;
  description: string;
  kind:
    | "metric_threshold"
    | "metric_deadman"
    | "heartbeat_stale"
    | "log_pattern"
    | "composite"
    | "synthetic_check";
  severity: "critical" | "high" | "medium" | "low";
  target_scope: "asset" | "group" | "global";
  cooldown_seconds: number;
  reopen_after_seconds: number;
  evaluation_interval_seconds: number;
  window_seconds: number;
  condition: Record<string, unknown>;
  labels?: Record<string, string>;
  metadata?: Record<string, string>;
};

export type AlertSilence = {
  id: string;
  matchers: Record<string, string>;
  reason?: string;
  created_by: string;
  starts_at: string;
  ends_at: string;
  created_at: string;
};

export type Incident = {
  id: string;
  title: string;
  severity: "critical" | "high" | "medium" | "low";
  status: "open" | "investigating" | "mitigated" | "resolved" | "closed";
  source: "manual" | "alert_auto";
  summary?: string;
  assignee?: string;
  root_cause?: string;
  action_items?: string[];
  lessons_learned?: string;
  created_at: string;
  updated_at: string;
  resolved_at?: string;
};

export type IncidentEvent = {
  id: string;
  incident_id: string;
  kind: string;
  title: string;
  detail?: string;
  source?: string;
  created_at: string;
};

export type EnrollmentToken = {
  id: string;
  label: string;
  expires_at: string;
  max_uses: number;
  use_count: number;
  created_at: string;
  revoked_at?: string;
};

export type AgentTokenSummary = {
  id: string;
  asset_id: string;
  status: "active" | "revoked";
  enrolled_via?: string;
  expires_at: string;
  last_used_at?: string;
  created_at: string;
  revoked_at?: string;
  device_fingerprint?: string;
};

export type Asset = {
  id: string;
  type: string;
  name: string;
  source: string;
  tags?: string[];
  group_id?: string;
  status: string;
  platform?: string;
  resource_class?: string;
  resource_kind?: string;
  attributes?: Record<string, unknown>;
  last_seen_at: string;
  metadata?: Record<string, string>;
};

export interface HopConfig {
  host: string;
  port: number;
  username: string;
  credential_profile_id: string;
}

export interface JumpChain {
  hops: HopConfig[];
}

export type Group = {
  id: string;
  name: string;
  slug: string;
  parent_group_id?: string;
  icon?: string;
  sort_order: number;
  timezone?: string;
  location?: string;
  latitude?: number;
  longitude?: number;
  metadata?: Record<string, string>;
  jump_chain?: JumpChain | null;
  created_at: string;
  updated_at: string;
};

export type GroupTreeNode = {
  group: Group;
  children: GroupTreeNode[];
  depth: number;
};

export type GroupReliability = {
  group: Group;
  score: number;
  grade: string;
  assets_total: number;
  assets_online: number;
  assets_stale: number;
  assets_offline: number;
  failed_actions: number;
  failed_updates: number;
  error_logs: number;
  warn_logs: number;
  dead_letters: number;
  maintenance_active: boolean;
  suppress_alerts: boolean;
  block_actions: boolean;
  block_updates: boolean;
};

export type GroupTimelineEvent = {
  id: string;
  kind: string;
  severity: "info" | "warn" | "error";
  title: string;
  summary?: string;
  source?: string;
  asset_id?: string;
  run_id?: string;
  timestamp: string;
};

export type GroupTimelineImpact = {
  total_events: number;
  error_events: number;
  warn_events: number;
  info_events: number;
  failed_actions: number;
  failed_updates: number;
  assets_stale: number;
  assets_offline: number;
  dead_letters: number;
};

export type GroupTimelineResponse = {
  generated_at: string;
  from: string;
  to: string;
  window: string;
  group: Group;
  impact: GroupTimelineImpact;
  reliability: GroupReliability;
  events: GroupTimelineEvent[];
};

export type MaintenanceWindow = {
  id: string;
  group_id: string;
  name: string;
  start_at: string;
  end_at: string;
  suppress_alerts: boolean;
  block_actions: boolean;
  block_updates: boolean;
  created_at: string;
  updated_at: string;
};

export type LinkSuggestion = {
  id: string;
  source_asset_id: string;
  target_asset_id: string;
  match_reason: string;
  confidence: number;
  status: 'pending' | 'accepted' | 'dismissed';
  created_at: string;
};

export type Edge = {
  id: string;
  source_asset_id: string;
  target_asset_id: string;
  relationship_type: string;
  direction: string;
  criticality: string;
  origin: 'auto' | 'manual' | 'suggested' | 'dismissed';
  confidence: number;
  match_signals?: Record<string, unknown>;
  metadata?: Record<string, string>;
  created_at: string;
  updated_at: string;
};

export type CompositeMember = {
  asset_id: string;
  role: 'primary' | 'facet';
  created_at: string;
};

export type Composite = {
  composite_id: string;
  members: CompositeMember[];
};

export type CompositeResolvedAsset = Asset & {
  facets?: Array<{ asset_id: string; source: string; type: string }>;
};

export type Proposal = Edge; // Proposals are just edges with origin='suggested'

export type CanonicalCapabilitySpec = {
  id: string;
  scope: string;
  stability?: string;
  supports_dry_run?: boolean;
  supports_async?: boolean;
  requires_target?: boolean;
  params_schema?: Record<string, unknown>;
};

export type CanonicalTemplateBinding = {
  resource_id: string;
  template_id: string;
  tabs?: string[];
  operations?: string[];
  updated_at: string;
};

export type CanonicalStatusPayload = {
  registry: {
    capabilities: CanonicalCapabilitySpec[];
    operations: Array<Record<string, unknown>>;
    metrics: Array<Record<string, unknown>>;
    events: Array<Record<string, unknown>>;
    templates: Array<Record<string, unknown>>;
  };
  providers: Array<Record<string, unknown>>;
  capabilitySets: Array<Record<string, unknown>>;
  templateBindings: Record<string, CanonicalTemplateBinding>;
  reconciliation: Array<Record<string, unknown>>;
};

export type LiveStatusResponse = {
  timestamp: string;
  summary: {
    servicesUp: number;
    servicesTotal: number;
    assetCount: number;
    staleAssetCount: number;
  };
  endpoints: EndpointStatus[];
  assets: Asset[];
  telemetryOverview: TelemetryOverviewAsset[];
};

export type StatusResponse = {
  timestamp: string;
  summary: {
    servicesUp: number;
    servicesTotal: number;
    connectorCount: number;
    groupCount: number;
    assetCount: number;
    sessionCount: number;
    auditCount: number;
    processedJobs: number;
    actionRunCount: number;
    updateRunCount: number;
    deadLetterCount: number;
    staleAssetCount: number;
    retentionError?: string;
  };
  endpoints: EndpointStatus[];
  connectors: ConnectorDescriptor[];
  groups: Group[];
  assets: Asset[];
  telemetryOverview: TelemetryOverviewAsset[];
  recentLogs: LogEvent[];
  logSources: LogSource[];
  groupReliability: GroupReliability[];
  actionRuns: ActionRun[];
  updatePlans: UpdatePlan[];
  updateRuns: UpdateRun[];
  deadLetters: DeadLetterEvent[];
  deadLetterAnalytics: DeadLetterAnalytics;
  sessions: Session[];
  recentCommands: Command[];
  recentAudit: AuditEvent[];
  canonical?: CanonicalStatusPayload;
};

export type TelemetryOverviewAsset = {
  asset_id: string;
  name: string;
  type: string;
  source: string;
  group_id?: string;
  status: string;
  platform?: string;
  last_seen_at: string;
  metrics: {
    cpu_used_percent?: number;
    memory_used_percent?: number;
    disk_used_percent?: number;
    temperature_celsius?: number;
    network_rx_bytes_per_sec?: number;
    network_tx_bytes_per_sec?: number;
  };
};

export type MetricPoint = {
  ts: number;
  value: number;
};

export type TelemetrySeries = {
  metric: string;
  unit: string;
  points: MetricPoint[];
  current?: number;
};

export type AssetTelemetryDetails = {
  asset: {
    id: string;
    name: string;
    type: string;
    source: string;
    group_id?: string;
    status: string;
    platform?: string;
    last_seen_at: string;
  };
  window: string;
  step: string;
  from: string;
  to: string;
  series: TelemetrySeries[];
};

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

export const themeOptions: Array<{ id: Theme; label: string }> = [
  { id: "oled", label: "OLED" },
  { id: "dark", label: "Dark" },
  { id: "light", label: "Light" },
];

export const densityOptions: Array<{ id: Density; label: string }> = [
  { id: "minimal", label: "Minimal" },
  { id: "diagnostic", label: "Diagnostic" },
];

export const telemetryWindows = ["15m", "1h", "6h", "24h"] as const;
export type TelemetryWindow = (typeof telemetryWindows)[number];
export const groupTimelineWindows = ["1h", "6h", "24h"] as const;
export type GroupTimelineWindow = (typeof groupTimelineWindows)[number];
export const logLevels = ["all", "debug", "info", "warn", "error"] as const;
export type LogLevel = (typeof logLevels)[number];
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

export {
  buildNodeMetadataSections,
  nodeMetadataFields,
  nodeMetadataSectionOrder,
  type NodeMetadataFieldSpec,
  type NodeMetadataRow,
  type NodeMetadataSection,
  type NodeMetadataSectionName,
} from "./nodeMetadata";

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

export type { AlertInstance, AlertRule, AlertRuleTemplate, AlertSilence, Incident, IncidentEvent } from "./models/alerts";

export type EnrollmentToken = {
  id: string;
  label: string;
  expires_at: string;
  max_uses: number;
  use_count: number;
  created_at: string;
  revoked_at?: string;
  scope: "asset" | "group" | "unplaced" | "unrestricted" | "legacy_revoked";
  asset_id?: string;
  allowed_group_id?: string;
  created_by?: string;
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
export { sourceLabels, runtimeSettingKeys, retentionFieldDefs } from "./models/runtimeSettings";
export type { RuntimeSettingEntry, RuntimeSettingsPayload, RetentionSettingsPayload } from "./models/runtimeSettings";

export {
  buildNodeMetadataSections,
  nodeMetadataFields,
  nodeMetadataSectionOrder,
  type NodeMetadataFieldSpec,
  type NodeMetadataRow,
  type NodeMetadataSection,
  type NodeMetadataSectionName,
} from "./nodeMetadata";

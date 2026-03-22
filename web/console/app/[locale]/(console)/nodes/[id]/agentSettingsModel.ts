export type AgentSettingEntry = {
  key: string;
  label: string;
  description: string;
  type: "string" | "int" | "bool" | "enum";
  default_value: string;
  global_value?: string;
  override_value?: string;
  state_value?: string;
  effective_value: string;
  source: string;
  min_int?: number;
  max_int?: number;
  allowed_values?: string[];
  restart_required?: boolean;
  hub_managed: boolean;
  local_only: boolean;
  drift?: boolean;
};

export type AgentSettingsState = {
  status: string;
  revision?: string;
  last_error?: string;
  updated_at?: string;
  applied_at?: string;
  restart_required?: boolean;
  allow_remote_overrides: boolean;
  fingerprint?: string;
  values?: Record<string, string>;
};

export type AgentVersionStatus = "up_to_date" | "update_available" | "unknown";

export type AgentSettingsPayload = {
  asset_id: string;
  connected: boolean;
  fingerprint?: string;
  agent_version?: string;
  latest_agent_version?: string;
  latest_agent_published_at?: string;
  agent_version_status?: AgentVersionStatus;
  agent_version_error?: string;
  agent_platform?: string;
  agent_arch?: string;
  settings: AgentSettingEntry[];
  state?: AgentSettingsState;
};

export type AgentSettingsHistoryEvent = {
  status: string;
  revision?: string;
  last_error?: string;
  updated_at?: string;
  applied_at?: string;
  restart_required?: boolean;
  fingerprint?: string;
};

export type AgentSettingsHistoryPayload = {
  events?: AgentSettingsHistoryEvent[];
};

export type DockerTestResponse = {
  ok?: boolean;
  status?: string;
  output?: string;
  endpoint?: string;
  message?: string;
  error?: string;
};

export type AgentUpdateResponse = {
  ok?: boolean;
  status?: string;
  summary?: string;
  output?: string;
  job_id?: string;
  force?: boolean;
  agent_disconnected_expected?: boolean;
  error?: string;
};

export const settingKeyDockerEnabled = "docker_enabled";
export const settingKeyDockerEndpoint = "docker_endpoint";
export const settingKeyFilesRootMode = "files_root_mode";

export const serviceDiscoverySettingKeys = new Set<string>([
  "services_discovery_docker_enabled",
  "services_discovery_proxy_enabled",
  "services_discovery_proxy_traefik_enabled",
  "services_discovery_proxy_caddy_enabled",
  "services_discovery_proxy_npm_enabled",
  "services_discovery_port_scan_enabled",
  "services_discovery_port_scan_include_listening",
  "services_discovery_port_scan_ports",
  "services_discovery_lan_scan_enabled",
  "services_discovery_lan_scan_cidrs",
  "services_discovery_lan_scan_ports",
  "services_discovery_lan_scan_max_hosts",
]);

export function isServiceDiscoverySetting(key: string): boolean {
  return serviceDiscoverySettingKeys.has(key);
}

export function formatTime(value?: string): string {
  if (!value) return "Never";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleString();
}

export function settingControlSourceStatus(source: string): string {
  switch (source) {
    case "hub-override":
      return "active";
    case "hub-default":
      return "pending";
    default:
      return "inactive";
  }
}

export function buildDraft(settings: AgentSettingEntry[]): Record<string, string> {
  const next: Record<string, string> = {};
  for (const setting of settings) {
    next[setting.key] = setting.effective_value;
  }
  return next;
}

export function getAgentVersionStatusLabel(status?: AgentVersionStatus): string {
  switch (status) {
    case "up_to_date":
      return "Up to date";
    case "update_available":
      return "Update available";
    default:
      return "Unknown";
  }
}

export function getAgentVersionStatusClass(status?: AgentVersionStatus): string {
  switch (status) {
    case "up_to_date":
      return "text-[var(--ok)]";
    case "update_available":
      return "text-[var(--warn)]";
    default:
      return "text-[var(--muted)]";
  }
}

export async function readJSON(response: Response): Promise<Record<string, unknown>> {
  try {
    const payload = (await response.json()) as Record<string, unknown>;
    return payload;
  } catch {
    return {};
  }
}

export function readError(payload: Record<string, unknown>): string | undefined {
  const value = payload["error"];
  return typeof value === "string" ? value : undefined;
}

export function readUpdatedAssetName(payload: Record<string, unknown>): string | undefined {
  const rawAsset = payload["asset"];
  if (!rawAsset || typeof rawAsset !== "object") return undefined;
  const name = (rawAsset as Record<string, unknown>)["name"];
  return typeof name === "string" ? name : undefined;
}

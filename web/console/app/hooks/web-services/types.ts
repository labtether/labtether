export interface WebServiceAltURL {
  id: string;
  web_service_id: string;
  url: string;
  source: string; // "auto" | "manual" | "suggestion_accepted"
  created_at: string;
}

export interface WebService {
  id: string;
  service_key: string;
  name: string;
  category: string;
  url: string;
  source: string;
  status: string;
  response_ms: number;
  container_id?: string;
  service_unit?: string;
  host_asset_id: string;
  icon_key: string;
  metadata?: Record<string, string>;
  health?: WebServiceHealthSummary;
  alt_urls?: WebServiceAltURL[];
}

export interface WebServiceHealthPoint {
  at: string;
  status: string;
  response_ms?: number;
}

export interface WebServiceHealthSummary {
  window: string;
  checks: number;
  up_checks: number;
  uptime_percent: number;
  last_checked_at?: string;
  last_change_at?: string;
  recent?: WebServiceHealthPoint[];
}

export interface WebServiceDiscoverySourceStat {
  enabled: boolean;
  duration_ms: number;
  services_found: number;
}

export interface WebServiceDiscoveryStats {
  collected_at: string;
  cycle_duration_ms: number;
  total_services: number;
  sources?: Record<string, WebServiceDiscoverySourceStat>;
  final_source_count?: Record<string, number>;
}

export interface WebServiceDiscoveryHostStat {
  host_asset_id: string;
  last_seen: string;
  discovery: WebServiceDiscoveryStats;
}

export interface URLGroupingSuggestion {
  id: string;
  base_service_url: string;
  base_service_name: string;
  base_icon_key: string;
  suggested_url: string;
  confidence: number;
}

export interface UseWebServicesOptions {
  host?: string;
  includeHidden?: boolean;
  pollInterval?: number;
  detailLevel?: "compact" | "full";
}

export interface ManualWebServiceInput {
  host_asset_id: string;
  name: string;
  category: string;
  url: string;
  icon_key?: string;
  metadata?: Record<string, string>;
}

export interface WebServiceOverrideInput {
  host_asset_id: string;
  service_id: string;
  name_override?: string;
  category_override?: string;
  url_override?: string;
  icon_key_override?: string;
  tags_override?: string;
  hidden: boolean;
}

export interface WebServiceOverride {
  host_asset_id: string;
  service_id: string;
  name_override?: string;
  category_override?: string;
  url_override?: string;
  icon_key_override?: string;
  tags_override?: string;
  hidden: boolean;
  updated_at?: string;
}

export interface ServiceCustomIconInput {
  name: string;
  data_url: string;
}

export interface ServiceCustomIconRenameInput {
  id: string;
  name: string;
}

export interface ServiceCustomIcon {
  id: string;
  name: string;
  data_url: string;
  created_at?: string;
  updated_at?: string;
}

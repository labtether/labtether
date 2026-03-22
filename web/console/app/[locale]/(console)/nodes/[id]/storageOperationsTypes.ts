import type { RiskState } from "./storageOperationsUtils";

export type ProxmoxZFSPool = {
  name?: string;
  size?: number;
  free?: number;
  alloc?: number;
  frag?: number;
  health?: string;
  dedup?: number;
};

export type ProxmoxStorageDetails = {
  zfs_pools?: ProxmoxZFSPool[];
  collector_id?: string;
  fetched_at?: string;
};

export type StorageInsightsSummary = {
  degraded_pools: number;
  hot_pools: number;
  predicted_full_lt_30d: number;
  scrub_overdue: number;
  stale_telemetry: number;
};

export type StorageInsightPool = {
  name: string;
  health?: string;
  size_bytes?: number;
  used_bytes?: number;
  free_bytes?: number;
  used_percent?: number;
  frag_percent?: number;
  dedup_ratio?: number;
  growth_bytes_7d?: number;
  forecast?: {
    days_to_80?: number;
    days_to_full?: number;
    confidence?: string;
  };
  scrub?: {
    last_completed_at?: string;
    overdue?: boolean;
  };
  snapshots?: {
    count?: number;
    bytes?: number;
  };
  dependent_workloads?: {
    vm_count?: number;
    ct_count?: number;
    vm_ids?: number[];
    ct_ids?: number[];
  };
  risk_score?: number;
  risk_state?: string;
  reasons?: string[];
  telemetry_stale?: boolean;
};

export type StorageInsightEvent = {
  timestamp?: string;
  severity?: string;
  message?: string;
  pool?: string;
  node?: string;
  upid?: string;
  task_type?: string;
  task_status?: string;
  exit_status?: string;
};

export type StorageInsightsResponse = {
  generated_at?: string;
  window?: string;
  asset_id?: string;
  node?: string;
  kind?: string;
  summary?: StorageInsightsSummary;
  pools?: StorageInsightPool[];
  events?: StorageInsightEvent[];
  warnings?: string[];
  error?: string;
};

export type RiskFilter = "all" | "degraded" | "hot" | "predicted" | "scrub" | "stale";

export type StorageRow = {
  key: string;
  poolName: string;
  typeLabel: string;
  health: string;
  usedPercent: number | null;
  freeBytes: number | null;
  allocBytes: number | null;
  fragPercent: number | null;
  dedupRatio: number | null;
  growthBytes7d: number | null;
  daysTo80: number | null;
  daysToFull: number | null;
  confidence: string;
  snapshotCount: number;
  snapshotBytes: number;
  vmCount: number;
  ctCount: number;
  vmIDs: number[];
  ctIDs: number[];
  stale: boolean;
  staleLabel: string;
  scrubOverdue: boolean;
  riskScore: number;
  riskState: RiskState;
  reason: string;
};

export type Recommendation = {
  key: string;
  rowKey: string;
  poolName: string;
  severity: "critical" | "warning" | "info";
  message: string;
  confidence: string;
  backupTarget?: {
    kind: "vm" | "ct";
    vmid: number;
  };
};

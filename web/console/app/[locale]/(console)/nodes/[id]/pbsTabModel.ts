export type PBSTask = {
  upid: string;
  node: string;
  worker_type: string;
  worker_id?: string;
  user?: string;
  starttime?: number;
  endtime?: number;
  status?: string;
};

export type PBSTaskStatus = {
  upid: string;
  node: string;
  type?: string;
  id?: string;
  user?: string;
  status?: string;
  exitstatus?: string;
  starttime?: number;
  pid?: number;
};

export type PBSTaskLogLine = {
  n: number;
  t: string;
};

export type PBSDatastoreSummary = {
  store: string;
  status: string;
  mount_status?: string;
  maintenance_mode?: string;
  comment?: string;
  total_bytes?: number;
  used_bytes?: number;
  avail_bytes?: number;
  usage_percent?: number;
  group_count?: number;
  snapshot_count?: number;
  last_backup_at?: string;
  days_since_backup?: number;
};

export type PBSAssetDetailsResponse = {
  asset_id?: string;
  kind?: string;
  collector_id?: string;
  node?: string;
  version?: string;
  store?: string;
  datastore?: PBSDatastoreSummary;
  datastores?: PBSDatastoreSummary[];
  tasks?: PBSTask[];
  warnings?: string[];
  fetched_at?: string;
  error?: string;
};

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function asString(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function asNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function asStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((entry): entry is string => typeof entry === "string");
}

export function normalizePBSAssetDetailsResponse(value: unknown): PBSAssetDetailsResponse {
  const raw = asRecord(value) ?? {};
  const datastore = asRecord(raw.datastore);
  return {
    asset_id: asString(raw.asset_id),
    kind: asString(raw.kind),
    collector_id: asString(raw.collector_id),
    node: asString(raw.node),
    version: asString(raw.version),
    store: asString(raw.store),
    datastore: datastore
      ? {
          store: asString(datastore.store) ?? "",
          status: asString(datastore.status) ?? "",
          mount_status: asString(datastore.mount_status),
          maintenance_mode: asString(datastore.maintenance_mode),
          comment: asString(datastore.comment),
          total_bytes: asNumber(datastore.total_bytes),
          used_bytes: asNumber(datastore.used_bytes),
          avail_bytes: asNumber(datastore.avail_bytes),
          usage_percent: asNumber(datastore.usage_percent),
          group_count: asNumber(datastore.group_count),
          snapshot_count: asNumber(datastore.snapshot_count),
          last_backup_at: asString(datastore.last_backup_at),
          days_since_backup: asNumber(datastore.days_since_backup),
        }
      : undefined,
    datastores: Array.isArray(raw.datastores)
      ? raw.datastores.map((entry) => {
          const store = asRecord(entry) ?? {};
          return {
            store: asString(store.store) ?? "",
            status: asString(store.status) ?? "",
            mount_status: asString(store.mount_status),
            maintenance_mode: asString(store.maintenance_mode),
            comment: asString(store.comment),
            total_bytes: asNumber(store.total_bytes),
            used_bytes: asNumber(store.used_bytes),
            avail_bytes: asNumber(store.avail_bytes),
            usage_percent: asNumber(store.usage_percent),
            group_count: asNumber(store.group_count),
            snapshot_count: asNumber(store.snapshot_count),
            last_backup_at: asString(store.last_backup_at),
            days_since_backup: asNumber(store.days_since_backup),
          };
        })
      : [],
    tasks: Array.isArray(raw.tasks)
      ? raw.tasks.map((entry) => {
          const task = asRecord(entry) ?? {};
          return {
            upid: asString(task.upid) ?? "",
            node: asString(task.node) ?? "",
            worker_type: asString(task.worker_type) ?? "",
            worker_id: asString(task.worker_id),
            user: asString(task.user),
            starttime: asNumber(task.starttime),
            endtime: asNumber(task.endtime),
            status: asString(task.status),
          };
        })
      : [],
    warnings: asStringArray(raw.warnings),
    fetched_at: asString(raw.fetched_at),
    error: asString(raw.error),
  };
}

export function normalizePBSTaskStatus(value: unknown): PBSTaskStatus | null {
  const raw = asRecord(value);
  if (!raw) return null;
  return {
    upid: asString(raw.upid) ?? "",
    node: asString(raw.node) ?? "",
    type: asString(raw.type),
    id: asString(raw.id),
    user: asString(raw.user),
    status: asString(raw.status),
    exitstatus: asString(raw.exitstatus),
    starttime: asNumber(raw.starttime),
    pid: asNumber(raw.pid),
  };
}

export function normalizePBSTaskLogLines(value: unknown): PBSTaskLogLine[] {
  if (!Array.isArray(value)) return [];
  return value.map((entry) => {
    const raw = asRecord(entry) ?? {};
    return {
      n: asNumber(raw.n) ?? 0,
      t: asString(raw.t) ?? "",
    };
  });
}

export function pbsStatusBadge(status?: string): "ok" | "pending" | "bad" {
  const normalized = (status ?? "").toLowerCase();
  if (normalized.includes("offline") || normalized.includes("error")) return "bad";
  if (normalized.includes("degraded") || normalized.includes("warn") || normalized.includes("stale")) return "pending";
  return "ok";
}

export function pbsTaskStatusBadge(status?: string): "ok" | "pending" | "bad" {
  const normalized = (status ?? "").toLowerCase();
  if (normalized.includes("error") || normalized.includes("fail") || normalized.includes("abort")) return "bad";
  if (normalized.includes("run") || normalized.includes("active")) return "pending";
  return "ok";
}

export function formatRelativeEpoch(epochSeconds: number): string {
  if (!Number.isFinite(epochSeconds) || epochSeconds <= 0) return "n/a";
  return formatRelativeTime(new Date(epochSeconds * 1000).toISOString());
}

export function formatRelativeTime(raw: string): string {
  const timestamp = new Date(raw).getTime();
  if (!Number.isFinite(timestamp)) return "n/a";
  const diffMs = Date.now() - timestamp;
  if (diffMs < 60_000) return `${Math.max(1, Math.floor(diffMs / 1000))}s ago`;
  if (diffMs < 3_600_000) return `${Math.floor(diffMs / 60_000)}m ago`;
  if (diffMs < 86_400_000) return `${Math.floor(diffMs / 3_600_000)}h ago`;
  return `${Math.floor(diffMs / 86_400_000)}d ago`;
}

export function formatBytes(bytes?: number): string {
  if (typeof bytes !== "number" || !Number.isFinite(bytes) || bytes < 0) return "n/a";
  if (bytes < 1) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const index = Math.min(units.length - 1, Math.max(0, Math.floor(Math.log(bytes) / Math.log(1024))));
  const value = bytes / Math.pow(1024, index);
  return `${value.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

// ---------------------------------------------------------------------------
// Groups
// ---------------------------------------------------------------------------

export type PBSBackupGroupEntry = {
  backup_type: string;
  backup_id: string;
  owner?: string;
  comment?: string;
  backup_count: number;
  last_backup?: number;
};

export type PBSDatastoreGroups = {
  store: string;
  groups: PBSBackupGroupEntry[];
};

export type PBSGroupsResponse = {
  datastores: PBSDatastoreGroups[];
  warnings?: string[];
  fetched_at?: string;
  error?: string;
};

// ---------------------------------------------------------------------------
// Snapshots
// ---------------------------------------------------------------------------

export type PBSSnapshotEntry = {
  backup_type: string;
  backup_id: string;
  backup_time: number;
  size?: number;
  protected?: boolean;
  owner?: string;
  comment?: string;
  verification?: { state?: string; upid?: string };
  files?: string[];
};

export type PBSSnapshotsResponse = {
  store: string;
  snapshots: PBSSnapshotEntry[];
  fetched_at?: string;
  error?: string;
};

// ---------------------------------------------------------------------------
// Verification
// ---------------------------------------------------------------------------

export type PBSDatastoreVerification = {
  store: string;
  verified_count: number;
  unverified_count: number;
  failed_count: number;
  last_verify_time?: number;
  status: string;
};

export type PBSVerificationResponse = {
  datastores: PBSDatastoreVerification[];
  warnings?: string[];
  fetched_at?: string;
  error?: string;
};

// ---------------------------------------------------------------------------
// Normalizers
// ---------------------------------------------------------------------------

export function normalizePBSGroupsResponse(value: unknown): PBSGroupsResponse {
  const raw = asRecord(value) ?? {};
  return {
    datastores: Array.isArray(raw.datastores)
      ? raw.datastores.map((entry) => {
          const ds = asRecord(entry) ?? {};
          return {
            store: asString(ds.store) ?? "",
            groups: Array.isArray(ds.groups)
              ? ds.groups.map((g) => {
                  const group = asRecord(g) ?? {};
                  return {
                    backup_type: asString(group.backup_type) ?? "",
                    backup_id: asString(group.backup_id) ?? "",
                    owner: asString(group.owner),
                    comment: asString(group.comment),
                    backup_count: asNumber(group.backup_count) ?? 0,
                    last_backup: asNumber(group.last_backup),
                  };
                })
              : [],
          };
        })
      : [],
    warnings: asStringArray(raw.warnings),
    fetched_at: asString(raw.fetched_at),
    error: asString(raw.error),
  };
}

export function normalizePBSSnapshotsResponse(value: unknown): PBSSnapshotsResponse {
  const raw = asRecord(value) ?? {};
  return {
    store: asString(raw.store) ?? "",
    snapshots: Array.isArray(raw.snapshots)
      ? raw.snapshots.map((entry) => {
          const snap = asRecord(entry) ?? {};
          const verification = asRecord(snap.verification);
          return {
            backup_type: asString(snap.backup_type) ?? "",
            backup_id: asString(snap.backup_id) ?? "",
            backup_time: asNumber(snap.backup_time) ?? 0,
            size: asNumber(snap.size),
            protected: snap.protected === true,
            owner: asString(snap.owner),
            comment: asString(snap.comment),
            verification: verification
              ? { state: asString(verification.state), upid: asString(verification.upid) }
              : undefined,
            files: Array.isArray(snap.files)
              ? snap.files.filter((f): f is string => typeof f === "string")
              : undefined,
          };
        })
      : [],
    fetched_at: asString(raw.fetched_at),
    error: asString(raw.error),
  };
}

export function normalizePBSVerificationResponse(value: unknown): PBSVerificationResponse {
  const raw = asRecord(value) ?? {};
  return {
    datastores: Array.isArray(raw.datastores)
      ? raw.datastores.map((entry) => {
          const ds = asRecord(entry) ?? {};
          return {
            store: asString(ds.store) ?? "",
            verified_count: asNumber(ds.verified_count) ?? 0,
            unverified_count: asNumber(ds.unverified_count) ?? 0,
            failed_count: asNumber(ds.failed_count) ?? 0,
            last_verify_time: asNumber(ds.last_verify_time),
            status: asString(ds.status) ?? "unknown",
          };
        })
      : [],
    warnings: asStringArray(raw.warnings),
    fetched_at: asString(raw.fetched_at),
    error: asString(raw.error),
  };
}

// ---------------------------------------------------------------------------
// Threshold helpers
// ---------------------------------------------------------------------------

export function backupStaleness(lastBackupEpoch?: number): "ok" | "warn" | "bad" {
  if (!lastBackupEpoch || lastBackupEpoch <= 0) return "bad";
  const hoursAgo = (Date.now() - lastBackupEpoch * 1000) / 3_600_000;
  if (hoursAgo > 72) return "bad";
  if (hoursAgo > 24) return "warn";
  return "ok";
}

export function usageThreshold(percent?: number): "ok" | "warn" | "bad" {
  if (typeof percent !== "number") return "ok";
  if (percent >= 95) return "bad";
  if (percent >= 80) return "warn";
  return "ok";
}

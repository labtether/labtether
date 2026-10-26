import type { TrueNASDisk } from "./TrueNASDisksTab";
import type { TrueNASReplicationTask } from "./TrueNASReplicationTab";
import type { TrueNASService } from "./TrueNASServicesTab";
import type { TrueNASVM } from "./TrueNASVMsTab";
import type { TrueNASListEndpoint, TrueNASOverviewData } from "./useTrueNASData";
import { normalizeDataset, normalizePool, normalizeSnapshot } from "./normalizeTrueNASStorage";
import { bool, identifier, number, record, requireBool, requireRecord, requireText, strings, text, timestamp, type Row } from "./responseValues";

function service(row: Row): TrueNASService {
  const state = row.state == null ? undefined : requireText(row.state, "service state");
  const knownState = state?.trim().toLowerCase();
  return {
    name: requireText(row.service ?? row.name, "service name"),
    state,
    running: state === undefined ? requireBool(row.running, "service running")
      : knownState === "running" ? true : knownState === "stopped" ? false : undefined,
    enabled: requireBool(row.enabled, "service enabled"), uptime: text(row.uptime),
  };
}

function vm(row: Row): TrueNASVM {
  return {
    id: identifier(row.id, "vm"), name: requireText(row.name, "vm name"),
    status: requireText(record(row.status)?.state ?? row.status, "vm status"),
    vcpus: number(row.vcpus), memory: number(row.memory), autostart: bool(row.autostart),
  };
}

function schedule(value: unknown): string | undefined {
  if (value == null) return undefined;
  if (typeof value === "string") return value;
  const cron = requireRecord(value, "replication schedule");
  const expression = ["minute", "hour", "dom", "month", "dow"]
    .map((key) => requireText(cron[key], `replication schedule ${key}`)).join(" ");
  const begin = text(cron.begin);
  const end = text(cron.end);
  return begin && end ? `${expression} (${begin}–${end})` : expression;
}

function replication(row: Row): TrueNASReplicationTask {
  const state = record(row.state);
  return {
    id: identifier(row.id, "replication"), name: requireText(row.name, "replication name"),
    direction: text(row.direction), source_datasets: strings(row.source_datasets),
    target_dataset: text(row.target_dataset), transport: text(row.transport), schedule: schedule(row.schedule),
    last_run: timestamp(row.last_run) ?? timestamp(state?.datetime),
    last_run_state: text(row.last_run_state) ?? text(state?.state),
    enabled: requireBool(row.enabled, "replication enabled"),
  };
}

function disk(row: Row, envelope: Row | undefined): TrueNASDisk {
  const name = requireText(row.name, "disk name");
  return {
    name, model: text(row.model), serial: text(row.serial), type: text(row.type), size: number(row.size),
    temperature_celsius: number(row.temperature_celsius) ?? number(record(envelope?.temperatures)?.[name]),
    smart_status: text(row.smart_status), pool: text(row.pool),
  };
}

export function normalizeTrueNASListRows(endpoint: TrueNASListEndpoint, items: unknown[], envelope?: unknown): unknown[] {
  return items.map((item) => {
    const row = requireRecord(item, endpoint);
    switch (endpoint) {
      case "pools": return normalizePool(row);
      case "datasets": return normalizeDataset(row);
      case "snapshots": return normalizeSnapshot(row);
      case "services": return service(row);
      case "vms": return vm(row);
      case "replication": return replication(row);
      case "disks": return disk(row, record(envelope));
      case "shares/smb": case "shares/nfs": return {
        id: identifier(row.id, "share"), path: requireText(row.path, "share path"),
        name: endpoint === "shares/smb" ? requireText(row.name, "share name") : undefined,
        enabled: requireBool(row.enabled, "share enabled"), comment: text(row.comment),
        networks: strings(row.networks), hosts: strings(row.hosts),
      };
    }
  });
}

export function normalizeTrueNASOverview(payload: unknown): TrueNASOverviewData {
  const row = requireRecord(payload, "overview");
  const system = row.system_info == null ? row : requireRecord(row.system_info, "system info");
  let storageUsed = number(row.storage_used_bytes);
  let storageTotal = number(row.storage_total_bytes);
  if (row.pools != null) {
    if (!Array.isArray(row.pools)) throw new Error("invalid overview pools response");
    const pools = row.pools.map((item) => requireRecord(item, "overview pool"));
    // Missing/offline pool sizes are unknown, not zero capacity.
    if (pools.length && pools.every((pool) => number(pool.size) !== undefined && number(pool.allocated) !== undefined)) {
      storageUsed = pools.reduce((sum, pool) => sum + number(pool.allocated)!, 0);
      storageTotal = pools.reduce((sum, pool) => sum + number(pool.size)!, 0);
    }
  }
  if (row.services != null && !Array.isArray(row.services)) throw new Error("invalid overview services response");
  if (row.alerts != null && !Array.isArray(row.alerts)) throw new Error("invalid overview alerts response");
  return {
    hostname: text(row.hostname) ?? text(system.hostname), version: text(system.version), model: text(system.model),
    uptime: text(system.uptime), cpu_cores: number(system.cores ?? system.cpu_cores),
    memory_bytes: number(system.physmem ?? system.memory_bytes), ecc_enabled: bool(system.ecc_memory ?? system.ecc_enabled),
    storage_used_bytes: storageUsed, storage_total_bytes: storageTotal,
    services: (row.services as unknown[] | null | undefined)?.map((item) => service(requireRecord(item, "service"))),
    alerts: (row.alerts as unknown[] | null | undefined)?.map((item) => {
      const alert = requireRecord(item, "alert");
      return {
        level: requireText(alert.level, "alert level"),
        // React renders this as text; provider HTML is never injected.
        message: requireText(alert.formatted ?? alert.text ?? alert.message, "alert message"),
        dismissed: requireBool(alert.dismissed, "alert dismissed"),
      };
    }),
  };
}

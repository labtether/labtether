import type { TrueNASDataset } from "./TrueNASDatasetsTab";
import type { TrueNASPool, TrueNASVdev } from "./TrueNASPoolsTab";
import type { TrueNASSnapshot } from "./TrueNASSnapshotsTab";
import { bool, identifier, number, record, requireRecord, requireText, text, timestamp, type Row } from "./responseValues";

function vdevs(value: unknown, depth = 0): TrueNASVdev[] {
  if (!Array.isArray(value) || depth > 32) throw new Error("invalid pool topology response");
  return value.map((item) => {
    const row = requireRecord(item, "vdev");
    return {
      name: requireText(text(row.name) ?? text(row.path) ?? text(row.guid) ?? text(row.type), "vdev name"),
      type: requireText(row.type, "vdev type"),
      status: requireText(row.status, "vdev status"),
      children: row.children === undefined ? undefined : vdevs(row.children, depth + 1),
    };
  });
}

function topology(value: unknown): TrueNASVdev[] | undefined {
  if (value == null) return undefined;
  if (Array.isArray(value)) return vdevs(value);
  const groups = requireRecord(value, "pool topology");
  return ["data", "log", "cache", "spare", "special", "dedup"]
    .flatMap((key) => groups[key] === undefined ? [] : vdevs(groups[key]));
}

export function normalizePool(row: Row): TrueNASPool {
  const scan = record(row.scan);
  // A resilver is not a completed scrub. Do not label its end time as one.
  const scrub = text(scan?.function)?.toUpperCase() === "SCRUB"
    && text(scan?.state)?.toUpperCase() === "FINISHED" ? timestamp(scan?.end_time) : undefined;
  return {
    id: identifier(row.id, "pool"), name: requireText(row.name, "pool name"),
    status: requireText(row.status, "pool status"),
    size: number(row.size), allocated: number(row.allocated), free: number(row.free),
    fragmentation: number(row.fragmentation), last_scrub: timestamp(row.last_scrub) ?? scrub,
    topology: topology(row.topology),
  };
}

export function normalizeDataset(row: Row): TrueNASDataset {
  return {
    // Never rebuild action IDs from a display label or normalized asset ID.
    id: requireText(typeof row.id === "string" ? row.id : undefined, "dataset id"),
    name: requireText(row.name, "dataset name"),
    used: number(row.used), available: number(row.available), quota: number(row.quota),
    compression: text(row.compression), mountpoint: text(row.mountpoint), readonly: bool(row.readonly),
  };
}

export function normalizeSnapshot(row: Row): TrueNASSnapshot {
  const id = requireText(typeof row.id === "string" ? row.id : undefined, "snapshot id");
  const separator = id.lastIndexOf("@");
  const props = record(row.properties);
  const creation = record(props?.creation);
  const seconds = number(creation?.rawvalue);
  const created = timestamp(row.created) ?? timestamp(creation?.parsed)
    ?? (seconds === undefined ? undefined : timestamp({ $date: seconds * 1000 }));
  return {
    id, name: requireText(row.name, "snapshot name"),
    dataset: requireText(row.dataset ?? (separator > 0 ? id.slice(0, separator) : undefined), "snapshot dataset"),
    snapshot_name: requireText(row.snapshot_name ?? (separator > 0 ? id.slice(separator + 1) : undefined), "snapshot name"),
    created, used: number(row.used ?? props?.used), referenced: number(row.referenced ?? props?.referenced),
  };
}

import type {
  ClusterStatusEntry,
  NetworkInterface,
  ProxmoxDetails,
} from "./nodeDetailTypes";

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

export function normalizeClusterStatusEntries(value: unknown): ClusterStatusEntry[] {
  if (!Array.isArray(value)) return [];
  return value.map((entry) => {
    const raw = asRecord(entry) ?? {};
    return {
      name: asString(raw.name),
      type: asString(raw.type),
      nodeid: asNumber(raw.nodeid),
      ip: asString(raw.ip),
      online: asNumber(raw.online),
      level: asString(raw.level),
      local: asNumber(raw.local),
      quorate: asNumber(raw.quorate),
      nodes: asNumber(raw.nodes),
      version: asNumber(raw.version),
    };
  });
}

export function normalizeNetworkInterfaces(value: unknown): NetworkInterface[] {
  if (!Array.isArray(value)) return [];
  return value.map((entry) => {
    const raw = asRecord(entry) ?? {};
    return {
      iface: asString(raw.iface),
      type: asString(raw.type),
      address: asString(raw.address),
      netmask: asString(raw.netmask),
      cidr: asString(raw.cidr),
      gateway: asString(raw.gateway),
      bridge_ports: asString(raw.bridge_ports),
      active: asNumber(raw.active),
      autostart: asNumber(raw.autostart),
      method: asString(raw.method),
      families: asStringArray(raw.families),
    };
  });
}

export function normalizeProxmoxDetails(value: unknown): ProxmoxDetails {
  const raw = asRecord(value) ?? {};
  const ha = asRecord(raw.ha);
  const cephStatus = asRecord(raw.ceph_status);
  const cephPgMap = asRecord(cephStatus?.pgmap);
  const cephMonMap = asRecord(cephStatus?.monmap);

  return {
    asset_id: asString(raw.asset_id),
    kind: asString(raw.kind),
    node: asString(raw.node),
    vmid: asString(raw.vmid),
    collector_id: asString(raw.collector_id),
    version: asString(raw.version),
    config: asRecord(raw.config) ?? undefined,
    snapshots: Array.isArray(raw.snapshots)
      ? raw.snapshots.map((entry) => {
          const snapshot = asRecord(entry) ?? {};
          return {
            name: asString(snapshot.name),
            description: asString(snapshot.description),
            parent: asString(snapshot.parent),
            snaptime: asNumber(snapshot.snaptime),
            vmstate: asString(snapshot.vmstate),
          };
        })
      : [],
    tasks: Array.isArray(raw.tasks)
      ? raw.tasks.map((entry) => {
          const task = asRecord(entry) ?? {};
          return {
            upid: asString(task.upid),
            node: asString(task.node),
            id: asString(task.id),
            type: asString(task.type),
            user: asString(task.user),
            status: asString(task.status),
            exitstatus: asString(task.exitstatus),
            starttime: asNumber(task.starttime),
            endtime: asNumber(task.endtime),
          };
        })
      : [],
    ha: ha
      ? {
          match: (() => {
            const match = asRecord(ha.match);
            if (!match) return undefined;
            return {
              sid: asString(match.sid),
              state: asString(match.state),
              status: asString(match.status),
              group: asString(match.group),
              node: asString(match.node),
              comment: asString(match.comment),
              max_restart: asNumber(match.max_restart),
              max_relocate: asNumber(match.max_relocate),
            };
          })(),
          resources: Array.isArray(ha.resources)
            ? ha.resources.map((entry) => {
                const resource = asRecord(entry) ?? {};
                return {
                  sid: asString(resource.sid),
                  state: asString(resource.state),
                  status: asString(resource.status),
                  group: asString(resource.group),
                  node: asString(resource.node),
                  comment: asString(resource.comment),
                  max_restart: asNumber(resource.max_restart),
                  max_relocate: asNumber(resource.max_relocate),
                };
              })
            : [],
        }
      : undefined,
    firewall_rules: Array.isArray(raw.firewall_rules)
      ? raw.firewall_rules.map((entry) => {
          const rule = asRecord(entry) ?? {};
          return {
            pos: asNumber(rule.pos) ?? 0,
            type: asString(rule.type) ?? "",
            action: asString(rule.action) ?? "",
            source: asString(rule.source),
            dest: asString(rule.dest),
            proto: asString(rule.proto),
            dport: asString(rule.dport),
            sport: asString(rule.sport),
            enable: asNumber(rule.enable) ?? 0,
            comment: asString(rule.comment),
            macro: asString(rule.macro),
            iface: asString(rule.iface),
          };
        })
      : [],
    backup_schedules: Array.isArray(raw.backup_schedules)
      ? raw.backup_schedules.map((entry) => {
          const schedule = asRecord(entry) ?? {};
          return {
            id: asString(schedule.id) ?? "",
            schedule: asString(schedule.schedule) ?? "",
            storage: asString(schedule.storage) ?? "",
            mode: asString(schedule.mode) ?? "",
            compress: asString(schedule.compress),
            enabled: asNumber(schedule.enabled) ?? 0,
            vmid: asString(schedule.vmid),
            exclude: asString(schedule.exclude),
            maxfiles: asNumber(schedule.maxfiles),
            pool: asString(schedule.pool),
            comment: asString(schedule.comment),
            node: asString(schedule.node),
          };
        })
      : [],
    ceph_status: cephStatus
      ? {
          health: (() => {
            const health = asRecord(cephStatus.health);
            return health ? { status: asString(health.status) } : undefined;
          })(),
          pgmap: cephPgMap
            ? {
                pgs_by_state: Array.isArray(cephPgMap.pgs_by_state)
                  ? cephPgMap.pgs_by_state.map((entry) => {
                      const pg = asRecord(entry) ?? {};
                      return {
                        state_name: asString(pg.state_name) ?? "",
                        count: asNumber(pg.count) ?? 0,
                      };
                    })
                  : [],
                data_bytes: asNumber(cephPgMap.data_bytes),
                bytes_used: asNumber(cephPgMap.bytes_used),
                bytes_avail: asNumber(cephPgMap.bytes_avail),
                bytes_total: asNumber(cephPgMap.bytes_total),
              }
            : undefined,
          monmap: cephMonMap
            ? {
                mons: Array.isArray(cephMonMap.mons)
                  ? cephMonMap.mons.map((entry) => {
                      const monitor = asRecord(entry) ?? {};
                      return {
                        name: asString(monitor.name) ?? "",
                        rank: asNumber(monitor.rank) ?? 0,
                      };
                    })
                  : [],
              }
            : undefined,
        }
      : undefined,
    ceph_osds: Array.isArray(raw.ceph_osds)
      ? raw.ceph_osds.map((entry) => {
          const osd = asRecord(entry) ?? {};
          return {
            id: asNumber(osd.id),
            name: asString(osd.name),
            host: asString(osd.host),
            status: asString(osd.status),
            crush_weight: asNumber(osd.crush_weight),
            device_class: asString(osd.device_class),
          };
        })
      : [],
    zfs_pools: Array.isArray(raw.zfs_pools)
      ? raw.zfs_pools.map((entry) => {
          const pool = asRecord(entry) ?? {};
          return {
            name: asString(pool.name),
            size: asNumber(pool.size),
            free: asNumber(pool.free),
            alloc: asNumber(pool.alloc),
            frag: asNumber(pool.frag),
            health: asString(pool.health),
            dedup: asNumber(pool.dedup),
          };
        })
      : [],
    storage_content: Array.isArray(raw.storage_content)
      ? raw.storage_content.map((entry) => {
          const item = asRecord(entry) ?? {};
          return {
            volid: asString(item.volid) ?? "",
            format: asString(item.format) ?? "",
            size: asNumber(item.size) ?? 0,
            ctime: asNumber(item.ctime),
            content: asString(item.content) ?? "",
            vmid: asNumber(item.vmid),
            notes: asString(item.notes),
          };
        })
      : [],
    warnings: asStringArray(raw.warnings),
    fetched_at: asString(raw.fetched_at),
    error: asString(raw.error),
  };
}

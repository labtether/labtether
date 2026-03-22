export type ProxmoxSnapshot = {
  name?: string;
  description?: string;
  parent?: string;
  snaptime?: number;
  vmstate?: string;
};

export type ProxmoxTask = {
  upid?: string;
  node?: string;
  id?: string;
  type?: string;
  user?: string;
  status?: string;
  exitstatus?: string;
  starttime?: number;
  endtime?: number;
};

export type ProxmoxHAResource = {
  sid?: string;
  state?: string;
  status?: string;
  group?: string;
  node?: string;
  comment?: string;
  max_restart?: number;
  max_relocate?: number;
};

export type ProxmoxCephStatus = {
  health?: { status?: string };
  pgmap?: {
    pgs_by_state?: Array<{ state_name: string; count: number }>;
    data_bytes?: number;
    bytes_used?: number;
    bytes_avail?: number;
    bytes_total?: number;
  };
  monmap?: {
    mons?: Array<{ name: string; rank: number }>;
  };
};

export type ProxmoxCephOSD = {
  id?: number;
  name?: string;
  host?: string;
  status?: string;
  crush_weight?: number;
  device_class?: string;
};

export type ProxmoxZFSPool = {
  name?: string;
  size?: number;
  free?: number;
  alloc?: number;
  frag?: number;
  health?: string;
  dedup?: number;
};

export type ProxmoxDetails = {
  asset_id?: string;
  kind?: string;
  node?: string;
  vmid?: string;
  collector_id?: string;
  version?: string;
  config?: Record<string, unknown>;
  snapshots?: ProxmoxSnapshot[];
  tasks?: ProxmoxTask[];
  ha?: {
    match?: ProxmoxHAResource;
    resources?: ProxmoxHAResource[];
  };
  firewall_rules?: Array<{
    pos: number;
    type: string;
    action: string;
    source?: string;
    dest?: string;
    proto?: string;
    dport?: string;
    sport?: string;
    enable: number;
    comment?: string;
    macro?: string;
    iface?: string;
  }>;
  backup_schedules?: Array<{
    id: string;
    schedule: string;
    storage: string;
    mode: string;
    compress?: string;
    enabled: number;
    vmid?: string;
    exclude?: string;
    maxfiles?: number;
    pool?: string;
    comment?: string;
    node?: string;
  }>;
  ceph_status?: ProxmoxCephStatus;
  ceph_osds?: ProxmoxCephOSD[];
  zfs_pools?: ProxmoxZFSPool[];
  storage_content?: Array<{
    volid: string;
    format: string;
    size: number;
    ctime?: number;
    content: string;
    vmid?: number;
    notes?: string;
  }>;
  warnings?: string[];
  fetched_at?: string;
  error?: string;
};

export type ClusterStatusEntry = {
  name?: string;
  type?: string;
  nodeid?: number;
  ip?: string;
  online?: number;
  level?: string;
  local?: number;
  quorate?: number;
  nodes?: number;
  version?: number;
};

export type NetworkInterface = {
  iface?: string;
  type?: string;
  address?: string;
  netmask?: string;
  cidr?: string;
  gateway?: string;
  bridge_ports?: string;
  active?: number;
  autostart?: number;
  method?: string;
  families?: string[];
};

export {
  normalizeClusterStatusEntries,
  normalizeNetworkInterfaces,
  normalizeProxmoxDetails,
} from "./nodeDetailNormalizers";

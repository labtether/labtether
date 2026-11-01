// Rows of the TrueNAS lists as the console reads them (after normalizeTrueNASResponse).

export type TrueNASVdev = {
  name: string;
  type: string;
  status: string;
  children?: TrueNASVdev[];
};

export type TrueNASPool = {
  id: string | number;
  name: string;
  status: string;
  size?: number;
  allocated?: number;
  free?: number;
  fragmentation?: number;
  last_scrub?: string;
  topology?: TrueNASVdev[];
};

export type TrueNASDataset = {
  id: string;
  name: string;
  used?: number;
  available?: number;
  quota?: number;
  compression?: string;
  mountpoint?: string;
  readonly?: boolean;
};

export type TrueNASSnapshot = {
  id: string;
  name: string;
  dataset: string;
  snapshot_name: string;
  created?: string;
  used?: number;
  referenced?: number;
};

export type TrueNASSMBShare = {
  id: string | number;
  name: string;
  path: string;
  enabled: boolean;
  comment?: string;
};

export type TrueNASNFSShare = {
  id: string | number;
  path: string;
  enabled: boolean;
  networks?: string[];
  hosts?: string[];
};

export type TrueNASService = {
  name: string;
  state?: string;
  running?: boolean;
  enabled: boolean;
  uptime?: string;
};

export type TrueNASVM = {
  id: string | number;
  name: string;
  status?: string;
  vcpus?: number;
  memory?: number;
  autostart?: boolean;
};

export type TrueNASReplicationTask = {
  id: string | number;
  name: string;
  direction?: string;
  source_datasets?: string[];
  target_dataset?: string;
  transport?: string;
  schedule?: string;
  last_run?: string;
  last_run_state?: string;
  enabled: boolean;
};

export type TrueNASDisk = {
  name: string;
  model?: string;
  serial?: string;
  type?: string;
  size?: number;
  temperature_celsius?: number;
  smart_status?: string;
  pool?: string;
};

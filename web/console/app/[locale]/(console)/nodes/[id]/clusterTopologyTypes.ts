import type { Asset } from "../../../../console/models";

export type ClusterStatusEntry = {
  type?: string;
  name?: string;
  nodeid?: number;
  ip?: string;
  online?: number;
  level?: string;
  quorate?: number;
  nodes?: number;
  version?: number;
  local?: number;
};

export type HAResource = {
  sid?: string;
  state?: string;
  status?: string;
  group?: string;
  type?: string;
  node?: string;
};

export type AssetDependency = {
  id: string;
  source_asset_id: string;
  target_asset_id: string;
  relationship_type: string;
  metadata?: Record<string, string>;
  updated_at?: string;
};

export type ProxmoxDetailsPayload = {
  config?: Record<string, unknown>;
};

export type LinkMode = "manual" | "auto";
export type TopologyView = "graph" | "list";

export interface ClusterTopologySectionProps {
  clusterStatus: ClusterStatusEntry[];
  haResources?: HAResource[];
  assets?: Asset[];
}

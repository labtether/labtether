import { Layers3, MapPin } from "lucide-react";
import { assetFreshnessLabel } from "../../../console/formatters";
import type { Asset } from "../../../console/models";
import {
  assetCategory,
  assetTypeIcon,
  friendlySourceLabel,
  friendlyTypeLabel,
  isInfraHost,
  sourceIcon,
} from "../../../console/taxonomy";

type StatusFilter = "all" | "online" | "unresponsive" | "offline" | "unknown";
type GraphDensity = "balanced" | "dense" | "ultra";
type LaneGrouping = "category" | "group" | "source" | "type";
type TopologyCategoryKey = "infra" | "compute" | "storage" | "network" | "services" | "power" | "other";

export type FreshnessCounts = {
  total: number;
  online: number;
  unresponsive: number;
  offline: number;
  unknown: number;
  issues: number;
};

const SOURCE_PRIORITY: Record<string, number> = {
  proxmox: 1,
  truenas: 2,
  docker: 3,
  portainer: 4,
  agent: 5,
};

const SERVICE_ASSET_TYPES = new Set([
  "service",
  "ha-entity",
  "compose-stack",
  "stack",
  "container",
  "docker-container",
]);
export const TOPOLOGY_CATEGORY_ORDER: TopologyCategoryKey[] = [
  "infra",
  "compute",
  "storage",
  "network",
  "services",
  "power",
  "other",
];

const TOPOLOGY_CATEGORY_LABELS: Record<TopologyCategoryKey, string> = {
  infra: "Infrastructure",
  compute: "Compute",
  storage: "Storage",
  network: "Network",
  services: "Services",
  power: "Power",
  other: "Other",
};

export const LANE_GROUPING_LABELS: Record<LaneGrouping, { title: string; singular: string; plural: string }> = {
  category: { title: "Category", singular: "category", plural: "categories" },
  group: { title: "Group", singular: "group", plural: "groups" },
  source: { title: "Source", singular: "source", plural: "sources" },
  type: { title: "Type", singular: "type", plural: "types" },
};

export function sourceRank(source: string): number {
  return SOURCE_PRIORITY[source.toLowerCase()] ?? 99;
}

export function topologyCategoryKey(asset: Asset): TopologyCategoryKey {
  if (isInfraHost(asset)) return "infra";
  if (isServiceAssetType(asset.type)) return "services";
  const mapped = assetCategory(asset.type);
  if (mapped === "compute" || mapped === "storage" || mapped === "network" || mapped === "services" || mapped === "power") {
    return mapped;
  }
  return "other";
}

export function laneKeyForAsset(asset: Asset, grouping: LaneGrouping): string {
  if (grouping === "category") return topologyCategoryKey(asset);
  if (grouping === "group") return asset.group_id || "unassigned";
  if (grouping === "type") return asset.type;
  return asset.source;
}

export function laneLabelForKey(laneKey: string, grouping: LaneGrouping, groupLabelByID: Map<string, string>): string {
  if (grouping === "category") return TOPOLOGY_CATEGORY_LABELS[(laneKey as TopologyCategoryKey)] ?? "Other";
  if (grouping === "group") return laneKey === "unassigned" ? "Unassigned" : (groupLabelByID.get(laneKey) ?? laneKey);
  if (grouping === "type") return friendlyTypeLabel(laneKey);
  return friendlySourceLabel(laneKey);
}

export function laneIconForKey(laneKey: string, grouping: LaneGrouping) {
  if (grouping === "category") return Layers3;
  if (grouping === "group") return MapPin;
  if (grouping === "type") return assetTypeIcon(laneKey);
  return sourceIcon(laneKey);
}

export function orderLaneKeys(keys: string[], grouping: LaneGrouping, groupLabelByID: Map<string, string>): string[] {
  return [...keys].sort((left, right) => {
    if (grouping === "category") {
      const leftIndex = TOPOLOGY_CATEGORY_ORDER.indexOf(left as TopologyCategoryKey);
      const rightIndex = TOPOLOGY_CATEGORY_ORDER.indexOf(right as TopologyCategoryKey);
      return leftIndex - rightIndex;
    }
    if (grouping === "group") {
      if (left === "unassigned") return 1;
      if (right === "unassigned") return -1;
      const leftLabel = groupLabelByID.get(left) ?? left;
      const rightLabel = groupLabelByID.get(right) ?? right;
      return leftLabel.localeCompare(rightLabel);
    }
    if (grouping === "source") {
      const rankDiff = sourceRank(left) - sourceRank(right);
      if (rankDiff !== 0) return rankDiff;
      return left.localeCompare(right);
    }
    const leftLabel = friendlyTypeLabel(left);
    const rightLabel = friendlyTypeLabel(right);
    return leftLabel.localeCompare(rightLabel);
  });
}

export function isServiceAssetType(type: string): boolean {
  return SERVICE_ASSET_TYPES.has(type.toLowerCase());
}

export function relationshipTone(
  relationshipType: string,
  inferred: boolean,
): { stroke: string; label: string; animated: boolean; chipClass: string } {
  if (inferred) {
    return {
      stroke: "#71717a",
      label: "hosted on",
      animated: false,
      chipClass: "bg-[var(--surface)] text-[var(--muted)]",
    };
  }

  const rel = relationshipType.toLowerCase();
  if (rel === "runs_on") {
    return {
      stroke: "#10b981",
      label: "runs on",
      animated: true,
      chipClass: "bg-[var(--ok-glow)] text-[var(--ok)]",
    };
  }
  if (rel === "hosted_on") {
    return {
      stroke: "#10b981",
      label: "hosted on",
      animated: false,
      chipClass: "bg-[var(--ok-glow)] text-[var(--ok)]",
    };
  }
  if (rel === "depends_on") {
    return {
      stroke: "#f59e0b",
      label: "depends on",
      animated: false,
      chipClass: "bg-[var(--warn-glow)] text-[var(--warn)]",
    };
  }
  if (rel === "provides_to") {
    return {
      stroke: "#3b82f6",
      label: "provides to",
      animated: false,
      chipClass: "bg-[var(--accent-subtle)] text-[var(--accent-text)]",
    };
  }
  if (rel === "contains") {
    return {
      stroke: "#8b5cf6",
      label: "contains",
      animated: false,
      chipClass: "bg-[var(--accent-subtle)] text-[var(--accent-text)]",
    };
  }

  return {
    stroke: "#a1a1aa",
    label: rel.replace(/_/g, " "),
    animated: false,
    chipClass: "bg-[var(--surface)] text-[var(--muted)]",
  };
}

export function relationshipPriority(relationshipType: string, inferred: boolean): number {
  const rel = relationshipType.toLowerCase();
  if (!inferred && rel === "runs_on") return 0;
  if (!inferred && rel === "hosted_on") return 1;
  if (!inferred && rel === "depends_on") return 2;
  if (!inferred && rel === "provides_to") return 3;
  if (!inferred && rel === "contains") return 4;
  if (!inferred) return 5;
  return 9;
}

export function assetFreshness(asset: Asset): Exclude<StatusFilter, "all"> {
  const freshness = assetFreshnessLabel(asset.last_seen_at);
  if (freshness === "online") return "online";
  if (freshness === "unresponsive") return "unresponsive";
  if (freshness === "offline") return "offline";
  return "unknown";
}

export function summarizeFreshness(assets: Asset[]): FreshnessCounts {
  let online = 0;
  let unresponsive = 0;
  let offline = 0;
  let unknown = 0;

  for (const asset of assets) {
    const freshness = assetFreshness(asset);
    if (freshness === "online") online += 1;
    else if (freshness === "unresponsive") unresponsive += 1;
    else if (freshness === "offline") offline += 1;
    else unknown += 1;
  }

  return {
    total: assets.length,
    online,
    unresponsive,
    offline,
    unknown,
    issues: unresponsive + offline + unknown,
  };
}

export function freshnessColor(asset: Asset): { border: string; dot: string; label: string; chip: string } {
  const freshness = assetFreshness(asset);
  if (freshness === "online") {
    return {
      border: "border-[var(--ok)]/45",
      dot: "bg-[var(--ok)]",
      label: "Online",
      chip: "bg-[var(--ok-glow)] text-[var(--ok)]",
    };
  }
  if (freshness === "unresponsive") {
    return {
      border: "border-[var(--warn)]/45",
      dot: "bg-[var(--warn)]",
      label: "Unresponsive",
      chip: "bg-[var(--warn-glow)] text-[var(--warn)]",
    };
  }
  if (freshness === "offline") {
    return {
      border: "border-[var(--bad)]/45",
      dot: "bg-[var(--bad)]",
      label: "Offline",
      chip: "bg-[var(--bad-glow)] text-[var(--bad)]",
    };
  }
  return {
    border: "border-[var(--muted)]/45",
    dot: "bg-[var(--muted)]",
    label: "Unknown",
    chip: "bg-[var(--surface)] text-[var(--muted)]",
  };
}

export function formatTimestamp(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return "Unknown";
  }
  return parsed.toLocaleString();
}

export function sourceLaneLayout(assetCount: number, density: GraphDensity): {
  cols: number;
  cardWidth: number;
  cardHeight: number;
  colGap: number;
  rowHeight: number;
  rowGap: number;
  topOffset: number;
  laneWidth: number;
} {
  const ultra = density === "ultra";
  const dense = density === "dense";
  const cols = ultra
    ? (assetCount >= 84 ? 7 : assetCount >= 60 ? 6 : assetCount >= 40 ? 5 : assetCount >= 24 ? 4 : assetCount >= 12 ? 3 : assetCount >= 6 ? 2 : 1)
    : dense
      ? (assetCount >= 48 ? 5 : assetCount >= 30 ? 4 : assetCount >= 16 ? 3 : assetCount >= 7 ? 2 : 1)
      : (assetCount >= 36 ? 4 : assetCount >= 20 ? 3 : assetCount >= 10 ? 2 : 1);

  const cardWidth = ultra
    ? (cols >= 7 ? 136 : cols === 6 ? 144 : cols === 5 ? 152 : cols === 4 ? 166 : cols === 3 ? 184 : cols === 2 ? 214 : 244)
    : dense
      ? (cols >= 5 ? 168 : cols === 4 ? 184 : cols === 3 ? 204 : cols === 2 ? 228 : 256)
      : (cols >= 4 ? 192 : cols === 3 ? 216 : cols === 2 ? 238 : 268);
  const cardHeight = ultra ? 84 : dense ? 96 : 110;
  const rowGap = ultra ? 12 : dense ? 16 : 20;

  const colGap = cardWidth + (ultra ? 10 : dense ? 14 : 20);
  const rowHeight = cardHeight + rowGap;
  const lanePadX = ultra ? 16 : dense ? 24 : 30;

  return {
    cols,
    cardWidth,
    cardHeight,
    colGap,
    rowHeight,
    rowGap,
    topOffset: 52,
    laneWidth: (cols * colGap) + lanePadX,
  };
}

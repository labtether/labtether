"use client";

import type { Asset,Group } from "../../../console/models";
import { type DeviceCardData,type Freshness } from "./nodesPageUtils";

// ── Tree node types ──

export type DeviceTreeItem =
  | {
      type: "group";
      id: string;
      group: Group;
      children: DeviceTreeItem[];
      depth: number;
      expanded: boolean;
      counts: { online: number; stale: number; offline: number };
    }
  | {
      type: "device";
      id: string;
      card: DeviceCardData;
      children: DeviceTreeItem[];
      depth: number;
      expanded: boolean;
      /** True when there are pending edge proposals involving this asset. */
      hasProposal?: boolean;
      /** Composite facet metadata for this device (from resolved composites). */
      facets?: Array<{ asset_id: string; source: string; type: string }>;
      /** Summary of edge-based children when collapsed (e.g., "2 VMs · 3 Containers"). */
      childSummary?: string;
    };

export function matchesQuery(asset: Asset, query: string): boolean {
  if (!query) return true;
  const terms: string[] = [
    asset.name, asset.type, asset.resource_kind ?? "",
    asset.source, asset.platform ?? "",
  ];
  terms.push(...(asset.tags ?? []));
  if (asset.metadata) {
    terms.push(...Object.values(asset.metadata));
  }
  return terms.join(" ").toLowerCase().includes(query);
}

export function countDeviceStatuses(items: DeviceTreeItem[]): { online: number; stale: number; offline: number } {
  let online = 0;
  let stale = 0;
  let offline = 0;
  for (const item of items) {
    if (item.type === "device") {
      const f = item.card.freshness;
      if (f === "online") online++;
      else if (f === "unresponsive") stale++;
      else offline++;
      // Count nested device children too
      const nested = countDeviceStatuses(item.children);
      online += nested.online;
      stale += nested.stale;
      offline += nested.offline;
    } else if (item.type === "group") {
      online += item.counts.online;
      stale += item.counts.stale;
      offline += item.counts.offline;
    }
  }
  return { online, stale, offline };
}

export function freshnessRank(freshness: Freshness): number {
  if (freshness === "offline") return 0;
  if (freshness === "unresponsive") return 1;
  if (freshness === "unknown") return 2;
  return 3;
}

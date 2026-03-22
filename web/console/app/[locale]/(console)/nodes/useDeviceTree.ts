"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Asset, CompositeResolvedAsset, Edge, Group, Proposal, TelemetryOverviewAsset } from "../../../console/models";
import {
  isHiddenAsset,
  isDeviceTier,
  isHomeAssistantHubAsset,
  isInfraHost,
  isInfraChild,
  hostParentKey,
  childParentKey,
  friendlyTypeLabel,
} from "../../../console/taxonomy";
import { assetFreshness, parsePercent, type DeviceCardData, type Freshness, type WorkloadSummary } from "./nodesPageUtils";

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

// ── Persistence key ──

const STORAGE_KEY = "labtether-device-tree-expanded";

function loadExpanded(): Set<string> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return new Set<string>();
    const parsed: unknown = JSON.parse(raw);
    if (Array.isArray(parsed)) return new Set(parsed.filter((v): v is string => typeof v === "string"));
  } catch { /* ignore */ }
  return new Set<string>();
}

function saveExpanded(set: Set<string>): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify([...set]));
  } catch { /* ignore */ }
}

// ── Search helper ──

function matchesQuery(asset: Asset, query: string): boolean {
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

// ── Status counting ──

function countDeviceStatuses(items: DeviceTreeItem[]): { online: number; stale: number; offline: number } {
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

// ── Hook ──

export function useDeviceTree(params: {
  groups: Group[];
  assets: Asset[];
  telemetryOverview: TelemetryOverviewAsset[];
  query: string;
  /** Edges from the edge store (containment relationships). */
  edges?: Edge[];
  /** Pending discovery proposals for indicator dots. */
  proposals?: Proposal[];
  /** Resolved composites: assets with facets annotation from resolve_composites=true. */
  resolvedAssets?: CompositeResolvedAsset[];
}): {
  tree: DeviceTreeItem[];
  toggleExpand: (id: string) => void;
  expandAll: () => void;
  collapseAll: () => void;
} {
  const { groups, assets, telemetryOverview, query, edges = [], proposals = [], resolvedAssets } = params;

  const [expandedSet, setExpandedSet] = useState<Set<string>>(() => loadExpanded());
  const initializedRef = useRef(false);
  const useDefaultExpansion = expandedSet.size === 0 && !initializedRef.current;

  // Save to localStorage on change
  useEffect(() => {
    saveExpanded(expandedSet);
  }, [expandedSet]);

  // Build card data for device-tier assets (reusing logic from nodesPageUtils)
  const { cardsByGroupID, allCardIDs } = useMemo(() => {
    const telemetryByID = new Map<string, TelemetryOverviewAsset>();
    for (const t of telemetryOverview) {
      telemetryByID.set(t.asset_id, t);
    }

    const deviceAssets: Asset[] = [];
    const nonDeviceAssets: Asset[] = [];
    for (const asset of assets) {
      const includeAsHomeAssistantHub = isHomeAssistantHubAsset(asset);
      if (isHiddenAsset(asset) && !includeAsHomeAssistantHub) continue;
      if (includeAsHomeAssistantHub || isDeviceTier(asset)) {
        deviceAssets.push(asset);
      } else {
        nonDeviceAssets.push(asset);
      }
    }

    // Detect merged Docker container-hosts
    const agentHostByID = new Map<string, Asset>();
    for (const device of deviceAssets) {
      if (device.source === "agent") agentHostByID.set(device.id, device);
    }
    const mergedContainerHosts = new Set<string>();
    const dockerHostByAgentID = new Map<string, Asset>();
    for (const device of deviceAssets) {
      if (
        device.source === "docker" &&
        device.type === "container-host" &&
        device.metadata?.agent_id &&
        agentHostByID.has(device.metadata.agent_id)
      ) {
        mergedContainerHosts.add(device.id);
        dockerHostByAgentID.set(device.metadata.agent_id, device);
      }
    }

    // Parent key lookup
    const deviceByParentKey = new Map<string, Asset>();
    for (const device of deviceAssets) {
      if (isInfraHost(device)) {
        const parentKey = hostParentKey(device);
        if (mergedContainerHosts.has(device.id) && device.metadata?.agent_id) {
          const agentHost = agentHostByID.get(device.metadata.agent_id);
          if (agentHost) {
            deviceByParentKey.set(parentKey, agentHost);
            continue;
          }
        }
        deviceByParentKey.set(parentKey, device);
      }
    }

    // Workload counts
    const workloadsByDeviceID = new Map<string, WorkloadSummary>();
    for (const child of nonDeviceAssets) {
      const pk = childParentKey(child);
      if (!pk) continue;
      const parent = deviceByParentKey.get(pk);
      if (!parent) continue;
      const summary = workloadsByDeviceID.get(parent.id) ?? { vms: 0, containers: 0, stacks: 0, datastores: 0, other: 0 };
      const type = child.type;
      if (type === "vm") summary.vms++;
      else if (type === "container" || type === "docker-container") summary.containers++;
      else if (type === "stack" || type === "compose-stack") summary.stacks++;
      else summary.other++;
      workloadsByDeviceID.set(parent.id, summary);
    }

    // Hosted-on
    const hostedOnByID = new Map<string, { id: string; name: string }>();
    for (const device of deviceAssets) {
      if (!isInfraChild(device) && device.type !== "vm") continue;
      const pk = childParentKey(device);
      if (!pk) continue;
      const parent = deviceByParentKey.get(pk);
      if (!parent || parent.id === device.id) continue;
      hostedOnByID.set(device.id, { id: parent.id, name: parent.name });
    }

    // Build cards, group by group_id
    const byGroup = new Map<string, DeviceCardData[]>();
    const ids: string[] = [];

    for (const device of deviceAssets) {
      if (mergedContainerHosts.has(device.id)) continue;

      const freshness = assetFreshness(device);
      const tele = telemetryByID.get(device.id);
      const cpu = tele?.metrics.cpu_used_percent ?? parsePercent(device.metadata?.cpu_used_percent ?? device.metadata?.cpu_percent);
      const mem = tele?.metrics.memory_used_percent ?? parsePercent(device.metadata?.memory_used_percent ?? device.metadata?.memory_percent);
      const disk = tele?.metrics.disk_used_percent ?? parsePercent(device.metadata?.disk_used_percent);
      const merged = dockerHostByAgentID.get(device.id);

      const card: DeviceCardData = {
        asset: device,
        freshness,
        cpu: typeof cpu === "number" ? cpu : null,
        mem: typeof mem === "number" ? mem : null,
        disk: typeof disk === "number" ? disk : null,
        workloads: workloadsByDeviceID.get(device.id) ?? { vms: 0, containers: 0, stacks: 0, datastores: 0, other: 0 },
        hostedOn: hostedOnByID.get(device.id) ?? null,
        dockerHost: merged ? { id: merged.id, name: merged.name } : null,
        haHub: null,
      };

      const key = device.group_id || "__ungrouped__";
      const list = byGroup.get(key) ?? [];
      list.push(card);
      byGroup.set(key, list);
      ids.push(device.id);
    }

    // Sort within each group
    for (const [, cards] of byGroup) {
      cards.sort((a, b) => {
        const fa = freshnessRank(a.freshness);
        const fb = freshnessRank(b.freshness);
        if (fa !== fb) return fa - fb;
        return a.asset.name.localeCompare(b.asset.name);
      });
    }

    return { cardsByGroupID: byGroup, allCardIDs: ids };
  }, [assets, telemetryOverview]);

  // Build edge-based parent→child map and proposal indicators
  const { edgeParentToChildren, proposalAssetIDs } = useMemo(() => {
    const CONTAINMENT_TYPES = new Set(["contains", "runs_on", "hosted_on"]);
    const EXCLUDED_ORIGINS = new Set(["suggested", "dismissed"]);

    // Build parent→child from containment edges
    const parentToChildren = new Map<string, Set<string>>();
    for (const edge of edges) {
      if (!CONTAINMENT_TYPES.has(edge.relationship_type)) continue;
      if (EXCLUDED_ORIGINS.has(edge.origin)) continue;
      const parentID = edge.source_asset_id;
      const childID = edge.target_asset_id;
      if (parentID === childID) continue;
      const children = parentToChildren.get(parentID) ?? new Set<string>();
      children.add(childID);
      parentToChildren.set(parentID, children);
    }

    // Assets that have pending proposals (suggested origin)
    const proposalIDs = new Set<string>();
    for (const proposal of proposals) {
      proposalIDs.add(proposal.source_asset_id);
      proposalIDs.add(proposal.target_asset_id);
    }

    return { edgeParentToChildren: parentToChildren, proposalAssetIDs: proposalIDs };
  }, [edges, proposals]);

  // Build a lookup from asset ID to its card (across all groups)
  const cardByAssetID = useMemo(() => {
    const map = new Map<string, DeviceCardData>();
    for (const [, cards] of cardsByGroupID) {
      for (const card of cards) {
        map.set(card.asset.id, card);
      }
    }
    return map;
  }, [cardsByGroupID]);

  // Build a lookup from primary asset ID to its composite facets (from resolve_composites).
  const facetsByAssetID = useMemo(() => {
    const map = new Map<string, Array<{ asset_id: string; source: string; type: string }>>();
    if (!resolvedAssets) return map;
    for (const ra of resolvedAssets) {
      if (ra.facets && ra.facets.length > 0) {
        map.set(ra.id, ra.facets);
      }
    }
    return map;
  }, [resolvedAssets]);

  // Build the tree
  const tree = useMemo(() => {
    // 1. Build group tree from flat groups list
    const groupByID = new Map<string, Group>();
    const childGroupsByParent = new Map<string, Group[]>();
    const rootGroups: Group[] = [];

    for (const g of groups) {
      groupByID.set(g.id, g);
    }
    for (const g of groups) {
      if (g.parent_group_id && groupByID.has(g.parent_group_id)) {
        const siblings = childGroupsByParent.get(g.parent_group_id) ?? [];
        siblings.push(g);
        childGroupsByParent.set(g.parent_group_id, siblings);
      } else {
        rootGroups.push(g);
      }
    }

    // Sort groups by sort_order then name
    const sortGroups = (arr: Group[]) =>
      arr.sort((a, b) => {
        if (a.sort_order !== b.sort_order) return a.sort_order - b.sort_order;
        return a.name.localeCompare(b.name);
      });

    sortGroups(rootGroups);
    for (const [, children] of childGroupsByParent) {
      sortGroups(children);
    }

    // Determine matching assets for search filtering
    const matchingAssetIDs = new Set<string>();
    const isFiltering = query.length > 0;
    if (isFiltering) {
      for (const [, cards] of cardsByGroupID) {
        for (const card of cards) {
          if (matchesQuery(card.asset, query)) {
            matchingAssetIDs.add(card.asset.id);
          }
        }
      }
    }

    // Collect IDs of groups that have matching descendants (for auto-expand)
    const groupsWithMatches = new Set<string>();
    if (isFiltering) {
      // For each matching asset, walk up the group chain
      for (const [groupKey, cards] of cardsByGroupID) {
        const hasMatch = cards.some((c) => matchingAssetIDs.has(c.asset.id));
        if (!hasMatch) continue;
        let currentGroupID: string | undefined = groupKey === "__ungrouped__" ? undefined : groupKey;
        while (currentGroupID) {
          groupsWithMatches.add(currentGroupID);
          const parent = groupByID.get(currentGroupID);
          currentGroupID = parent?.parent_group_id ?? undefined;
        }
      }
    }

    // Build a child summary string (e.g., "2 VMs · 3 Containers") from edge children
    function buildChildSummary(childCards: DeviceCardData[]): string | undefined {
      if (childCards.length === 0) return undefined;
      const typeCounts = new Map<string, number>();
      for (const child of childCards) {
        const label = friendlyTypeLabel(
          (child.asset.resource_kind || child.asset.type || "").trim(),
        );
        typeCounts.set(label, (typeCounts.get(label) ?? 0) + 1);
      }
      const parts = Array.from(typeCounts.entries()).map(
        ([label, count]) => `${count} ${label}${count !== 1 ? "s" : ""}`,
      );
      return parts.length > 0 ? parts.join(" \u00b7 ") : undefined;
    }

    // Build a device tree node, nesting edge-based children one level deep
    function buildDeviceNode(card: DeviceCardData, depth: number): DeviceTreeItem {
      const assetID = card.asset.id;
      const edgeChildIDs = edgeParentToChildren.get(assetID);
      const childDeviceNodes: DeviceTreeItem[] = [];
      const childCards: DeviceCardData[] = [];

      if (edgeChildIDs && isInfraHost(card.asset)) {
        for (const childID of edgeChildIDs) {
          const childCard = cardByAssetID.get(childID);
          if (!childCard) continue;
          if (isFiltering && !matchingAssetIDs.has(childID)) continue;
          childCards.push(childCard);
          childDeviceNodes.push({
            type: "device",
            id: childCard.asset.id,
            card: childCard,
            children: [], // Shallow — only 1 level
            depth: depth + 1,
            expanded: false,
            hasProposal: proposalAssetIDs.has(childCard.asset.id),
          });
        }
      }

      // Use the raw asset ID in the expanded set — no collision with group IDs
      // which use "group:" prefix.
      const isExpanded = childDeviceNodes.length > 0 && expandedSet.has(assetID);

      return {
        type: "device",
        id: assetID,
        card,
        children: childDeviceNodes,
        depth,
        expanded: isExpanded,
        hasProposal: proposalAssetIDs.has(assetID),
        facets: facetsByAssetID.get(assetID),
        childSummary: buildChildSummary(childCards),
      };
    }

    // Track which asset IDs are nested as edge children so they are not duplicated at top level
    const edgeChildAssetIDs = new Set<string>();
    for (const [, childIDs] of edgeParentToChildren) {
      for (const childID of childIDs) {
        // Only suppress if the parent is actually a device-tier infra host in our card set
        edgeChildAssetIDs.add(childID);
      }
    }

    // Build group tree node recursively
    function buildGroupNode(group: Group, depth: number): DeviceTreeItem | null {
      const childGroups = childGroupsByParent.get(group.id) ?? [];
      const deviceCards = cardsByGroupID.get(group.id) ?? [];

      // Build children
      const childGroupNodes: DeviceTreeItem[] = [];
      for (const cg of childGroups) {
        const node = buildGroupNode(cg, depth + 1);
        if (node) childGroupNodes.push(node);
      }

      // Build device nodes — skip assets that are edge-children (they nest under their parent)
      const deviceNodes: DeviceTreeItem[] = [];
      for (const card of deviceCards) {
        if (isFiltering && !matchingAssetIDs.has(card.asset.id)) continue;
        // Skip edge children that have a parent in the card set
        if (edgeChildAssetIDs.has(card.asset.id) && !isInfraHost(card.asset)) {
          // Verify the parent actually exists in our cards — otherwise don't suppress
          let parentExists = false;
          for (const [parentID, childIDs] of edgeParentToChildren) {
            if (childIDs.has(card.asset.id) && cardByAssetID.has(parentID)) {
              parentExists = true;
              break;
            }
          }
          if (parentExists) continue;
        }
        deviceNodes.push(buildDeviceNode(card, depth + 1));
      }

      const allChildren: DeviceTreeItem[] = [...childGroupNodes, ...deviceNodes];

      // In filter mode, skip groups with no matching descendants
      if (isFiltering && allChildren.length === 0) return null;

      const counts = countDeviceStatuses(allChildren);
      const isExpanded = isFiltering
        ? groupsWithMatches.has(group.id)
        : useDefaultExpansion || expandedSet.has(`group:${group.id}`);

      return {
        type: "group",
        id: `group:${group.id}`,
        group,
        children: allChildren,
        depth,
        expanded: isExpanded,
        counts,
      };
    }

    const treeItems: DeviceTreeItem[] = [];

    // Build root group nodes
    for (const g of rootGroups) {
      const node = buildGroupNode(g, 0);
      if (node) treeItems.push(node);
    }

    // Ungrouped assets
    const ungroupedCards = cardsByGroupID.get("__ungrouped__") ?? [];
    const filteredUngrouped = isFiltering
      ? ungroupedCards.filter((c) => matchingAssetIDs.has(c.asset.id))
      : ungroupedCards;

    // Filter out edge children from ungrouped too
    const actualUngrouped = filteredUngrouped.filter((card) => {
      if (!edgeChildAssetIDs.has(card.asset.id) || isInfraHost(card.asset)) return true;
      for (const [parentID, childIDs] of edgeParentToChildren) {
        if (childIDs.has(card.asset.id) && cardByAssetID.has(parentID)) return false;
      }
      return true;
    });

    if (actualUngrouped.length > 0) {
      const ungroupedDevices: DeviceTreeItem[] = actualUngrouped.map((card) =>
        buildDeviceNode(card, 1),
      );

      const counts = countDeviceStatuses(ungroupedDevices);
      treeItems.push({
        type: "group",
        id: "group:__ungrouped__",
        group: {
          id: "__ungrouped__",
          name: "Ungrouped",
          slug: "ungrouped",
          sort_order: 999999,
          created_at: "",
          updated_at: "",
        },
        children: ungroupedDevices,
        depth: 0,
        expanded: isFiltering || useDefaultExpansion || expandedSet.has("group:__ungrouped__"),
        counts,
      });
    }

    // If no groups at all, show all devices flat
    if (groups.length === 0 && treeItems.length === 0) {
      const allCards: DeviceCardData[] = [];
      for (const [, cards] of cardsByGroupID) {
        for (const card of cards) {
          if (isFiltering && !matchingAssetIDs.has(card.asset.id)) continue;
          // Skip edge children
          if (edgeChildAssetIDs.has(card.asset.id) && !isInfraHost(card.asset)) {
            let parentExists = false;
            for (const [parentID, childIDs] of edgeParentToChildren) {
              if (childIDs.has(card.asset.id) && cardByAssetID.has(parentID)) {
                parentExists = true;
                break;
              }
            }
            if (parentExists) continue;
          }
          allCards.push(card);
        }
      }
      for (const card of allCards) {
        treeItems.push(buildDeviceNode(card, 0));
      }
    }

    return treeItems;
  }, [groups, cardsByGroupID, query, expandedSet, useDefaultExpansion, edgeParentToChildren, proposalAssetIDs, cardByAssetID, facetsByAssetID]);

  // Auto-expand all groups on first render if no saved state
  useEffect(() => {
    if (initializedRef.current || tree.length === 0) return;
    initializedRef.current = true;

    if (expandedSet.size > 0) return; // User has saved preferences

    // Expand all group nodes by default
    const allGroupIDs = new Set<string>();
    function collectGroupIDs(items: DeviceTreeItem[]) {
      for (const item of items) {
        if (item.type === "group") {
          allGroupIDs.add(item.id);
          collectGroupIDs(item.children);
        }
      }
    }
    collectGroupIDs(tree);
    if (allGroupIDs.size > 0) {
      setExpandedSet(allGroupIDs);
    }
  }, [tree, expandedSet.size]);

  const toggleExpand = useCallback((id: string) => {
    setExpandedSet((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const expandAll = useCallback(() => {
    const allIDs = new Set<string>();
    function collect(items: DeviceTreeItem[]) {
      for (const item of items) {
        if (item.type === "group" || item.children.length > 0) {
          allIDs.add(item.id);
        }
        collect(item.children);
      }
    }
    collect(tree);
    setExpandedSet(allIDs);
  }, [tree]);

  const collapseAll = useCallback(() => {
    setExpandedSet(new Set());
  }, []);

  return { tree, toggleExpand, expandAll, collapseAll };
}

// ── Helpers ──

function freshnessRank(freshness: Freshness): number {
  if (freshness === "offline") return 0;
  if (freshness === "unresponsive") return 1;
  if (freshness === "unknown") return 2;
  return 3;
}

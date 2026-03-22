"use client";

import { useState, useMemo, useCallback, memo } from "react";
import type { TopologyState, Zone } from "./topologyCanvasTypes";
import { STATUS_COLORS } from "./topologyCanvasTypes";
import { useFastStatus } from "../../../contexts/StatusContext";
import type { Asset } from "../../../console/models";
import { buildDisplayConnections } from "./topologyRelationshipGraph";

// ── Zone color text — matches the ZONE_COLORS map in ZoneNode.tsx ──────────
const ZONE_TEXT_COLORS: Record<string, string> = {
  blue:   "#7dd3fc",
  green:  "#86efac",
  purple: "#c4b5fd",
  amber:  "#fcd34d",
  rose:   "#fda4af",
  cyan:   "#67e8f9",
};

// ── Internal tree node types ─────────────────────────────────────────────────
type TreeNodeKind = "zone" | "asset" | "child";

interface ZoneTreeNode {
  kind: "zone";
  id: string;
  zone: Zone;
  assetCount: number;
  children: AssetTreeNode[];
  subZones: ZoneTreeNode[];
}

interface AssetTreeNode {
  kind: "asset";
  id: string;
  asset: Asset;
  discoveredChildren: ChildTreeNode[];
}

interface ChildTreeNode {
  kind: "child";
  id: string;
  asset: Asset;
}

type AnyTreeNode = ZoneTreeNode | AssetTreeNode | ChildTreeNode;

// ── Rendered flat row type ───────────────────────────────────────────────────
interface FlatRow {
  key: string;
  node: AnyTreeNode;
  depth: number;
  hasChildren: boolean;
}

// ── Build the zone hierarchy recursively ────────────────────────────────────
function buildZoneTree(
  zones: Zone[],
  parentID: string | null,
  membersByZone: Map<string, string[]>,
  assetsByID: Map<string, Asset>,
  childrenByAsset: Map<string, Asset[]>,
): ZoneTreeNode[] {
  return zones
    .filter((z) => z.parent_zone_id === parentID)
    .sort((a, b) => a.sort_order - b.sort_order)
    .map((zone) => {
      const memberIDs = membersByZone.get(zone.id) ?? [];
      const assetNodes: AssetTreeNode[] = memberIDs
        .map((aid) => {
          const asset = assetsByID.get(aid);
          if (!asset) return null;
          const discovered = childrenByAsset.get(aid) ?? [];
          return {
            kind: "asset" as const,
            id: `asset-${aid}`,
            asset,
            discoveredChildren: discovered.map((c) => ({
              kind: "child" as const,
              id: `child-${c.id}`,
              asset: c,
            })),
          };
        })
        .filter((n): n is AssetTreeNode => n !== null);

      const subZones = buildZoneTree(
        zones,
        zone.id,
        membersByZone,
        assetsByID,
        childrenByAsset,
      );

      const totalAssets = assetNodes.length + subZones.reduce((s, z) => s + z.assetCount, 0);

      return {
        kind: "zone" as const,
        id: `zone-${zone.id}`,
        zone,
        assetCount: totalAssets,
        children: assetNodes,
        subZones,
      };
    });
}

// ── Flatten the tree into rows, respecting collapse state ───────────────────
function flattenTree(
  nodes: (ZoneTreeNode | AssetTreeNode)[],
  depth: number,
  expandedIDs: Set<string>,
  rows: FlatRow[],
): void {
  for (const node of nodes) {
    if (node.kind === "zone") {
      const hasChildren = node.children.length > 0 || node.subZones.length > 0;
      rows.push({ key: node.id, node, depth, hasChildren });
      if (expandedIDs.has(node.id) && hasChildren) {
        flattenTree(node.subZones, depth + 1, expandedIDs, rows);
        flattenTree(node.children, depth + 1, expandedIDs, rows);
      }
    } else {
      const hasChildren = node.discoveredChildren.length > 0;
      rows.push({ key: node.id, node, depth, hasChildren });
      if (expandedIDs.has(node.id) && hasChildren) {
        for (const child of node.discoveredChildren) {
          rows.push({ key: child.id, node: child, depth: depth + 1, hasChildren: false });
        }
      }
    }
  }
}

// ── Props ────────────────────────────────────────────────────────────────────
export interface TopologyTreeViewProps {
  topology: TopologyState;
  selectedAssetID: string | null;
  onAssetSelect?: (assetID: string | null) => void;
}

// ── Component ────────────────────────────────────────────────────────────────
export default function TopologyTreeView({
  topology,
  selectedAssetID,
  onAssetSelect,
}: TopologyTreeViewProps) {
  const fastStatus = useFastStatus();
  const allAssets = useMemo(() => fastStatus?.assets ?? [], [fastStatus?.assets]);

  const assetsByID = useMemo(() => {
    const map = new Map<string, Asset>();
    for (const a of allAssets) map.set(a.id, a);
    return map;
  }, [allAssets]);

  const displayConnections = useMemo(
    () => buildDisplayConnections(allAssets, topology.connections),
    [allAssets, topology.connections],
  );

  const childrenByAsset = useMemo(() => {
    const map = new Map<string, Asset[]>();
    for (const connection of displayConnections) {
      if (connection.relationship !== "hosted_on" && connection.relationship !== "runs_on") {
        continue;
      }
      const childAsset = assetsByID.get(connection.source_asset_id);
      if (!childAsset) {
        continue;
      }
      const existing = map.get(connection.target_asset_id) ?? [];
      if (!existing.some((candidate) => candidate.id === childAsset.id)) {
        existing.push(childAsset);
        map.set(connection.target_asset_id, existing);
      }
    }
    return map;
  }, [displayConnections, assetsByID]);

  const nestedChildIDs = useMemo(() => {
    const visibleParentIDs = new Set<string>([
      ...topology.members.map((member) => member.asset_id),
      ...topology.unsorted,
    ]);
    const ids = new Set<string>();
    for (const connection of displayConnections) {
      if (connection.relationship !== "hosted_on" && connection.relationship !== "runs_on") {
        continue;
      }
      if (!visibleParentIDs.has(connection.target_asset_id)) {
        continue;
      }
      ids.add(connection.source_asset_id);
    }
    return ids;
  }, [displayConnections, topology.members, topology.unsorted]);

  // Members by zone_id
  const membersByZone = useMemo(() => {
    const map = new Map<string, string[]>();
    for (const m of topology.members) {
      if (nestedChildIDs.has(m.asset_id)) {
        continue;
      }
      const list = map.get(m.zone_id) ?? [];
      list.push(m.asset_id);
      map.set(m.zone_id, list);
    }
    return map;
  }, [topology.members, nestedChildIDs]);

  // Build the tree
  const zoneTree = useMemo(
    () => buildZoneTree(topology.zones, null, membersByZone, assetsByID, childrenByAsset),
    [topology.zones, membersByZone, assetsByID, childrenByAsset],
  );

  // Unsorted assets (not in any zone)
  const unsortedAssetNodes = useMemo<AssetTreeNode[]>(() => {
    return topology.unsorted
      .filter((assetID) => !nestedChildIDs.has(assetID))
      .map((aid) => {
        const asset = assetsByID.get(aid);
        if (!asset) return null;
        const discovered = childrenByAsset.get(aid) ?? [];
        return {
          kind: "asset" as const,
          id: `asset-${aid}`,
          asset,
          discoveredChildren: discovered.map((c) => ({
            kind: "child" as const,
            id: `child-${c.id}`,
            asset: c,
          })),
        };
      })
      .filter((n): n is AssetTreeNode => n !== null);
  }, [topology.unsorted, assetsByID, childrenByAsset, nestedChildIDs]);

  // Expand/collapse state: zones start expanded, assets start collapsed
  const [expandedIDs, setExpandedIDs] = useState<Set<string>>(() => {
    const s = new Set<string>();
    for (const z of topology.zones) s.add(`zone-${z.id}`);
    return s;
  });

  const [searchQuery, setSearchQuery] = useState("");

  const toggleExpanded = useCallback((id: string) => {
    setExpandedIDs((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  // Flatten all rows (zone tree + unsorted section)
  const allRows = useMemo<FlatRow[]>(() => {
    const rows: FlatRow[] = [];
    flattenTree(zoneTree, 0, expandedIDs, rows);
    // Unsorted pseudo-zone
    if (unsortedAssetNodes.length > 0) {
      const unsortedZoneKey = "__unsorted__";
      const isExpanded = expandedIDs.has(unsortedZoneKey);
      rows.push({
        key: unsortedZoneKey,
        node: {
          kind: "zone" as const,
          id: unsortedZoneKey,
          zone: {
            id: unsortedZoneKey,
            topology_id: topology.id,
            parent_zone_id: null,
            label: "Unsorted",
            color: "",
            icon: "",
            position: { x: 0, y: 0 },
            size: { width: 0, height: 0 },
            collapsed: false,
            sort_order: 9999,
          },
          assetCount: unsortedAssetNodes.length,
          children: unsortedAssetNodes,
          subZones: [],
        },
        depth: 0,
        hasChildren: unsortedAssetNodes.length > 0,
      });
      if (isExpanded) {
        flattenTree(unsortedAssetNodes, 1, expandedIDs, rows);
      }
    }
    return rows;
  }, [zoneTree, unsortedAssetNodes, expandedIDs, topology.id]);

  // Apply search filter — when an asset matches, include all ancestor zone rows too
  const filteredRows = useMemo<FlatRow[]>(() => {
    if (!searchQuery.trim()) return allRows;
    const q = searchQuery.toLowerCase();

    // Build a parent-key lookup from the flat rows: child key → parent zone key
    // We derive parentage by tracking the last seen zone key at each depth level.
    const parentKeyByDepth = new Map<number, string>();
    const parentOf = new Map<string, string>();
    for (const row of allRows) {
      if (row.node.kind === "zone") {
        parentKeyByDepth.set(row.depth, row.key);
        // Any deeper zones' children belong under this zone
      } else {
        // asset/child row: its parent is the zone at (depth - 1) or depth
        // Zones are always at a lower depth than their asset children
        const parentKey = parentKeyByDepth.get(row.depth - 1) ?? parentKeyByDepth.get(row.depth);
        if (parentKey) parentOf.set(row.key, parentKey);
      }
      // Also track sub-zone parentage
      if (row.node.kind === "zone" && row.depth > 0) {
        const parentKey = parentKeyByDepth.get(row.depth - 1);
        if (parentKey) parentOf.set(row.key, parentKey);
      }
    }

    // First pass: find all directly matching rows
    const mustShow = new Set<string>();
    const ancestorsToExpand = new Set<string>();
    for (const row of allRows) {
      let matches = false;
      if (row.node.kind === "zone") {
        matches = row.node.zone.label.toLowerCase().includes(q);
      } else {
        matches = row.node.asset.name.toLowerCase().includes(q);
      }
      if (matches) {
        mustShow.add(row.key);
        // Walk up ancestors
        let key: string | undefined = row.key;
        while (key) {
          const ancestor = parentOf.get(key);
          if (ancestor) {
            mustShow.add(ancestor);
            ancestorsToExpand.add(ancestor);
          }
          key = ancestor;
        }
      }
    }

    // Auto-expand ancestors so matching rows are actually visible
    if (ancestorsToExpand.size > 0) {
      setExpandedIDs((prev) => {
        let changed = false;
        const next = new Set(prev);
        for (const id of ancestorsToExpand) {
          if (!next.has(id)) { next.add(id); changed = true; }
        }
        return changed ? next : prev;
      });
    }

    return allRows.filter((row) => mustShow.has(row.key));
  }, [allRows, searchQuery]);

  const handleRowClick = useCallback(
    (row: FlatRow) => {
      if (row.node.kind === "zone") {
        toggleExpanded(row.node.id);
        return;
      }
      if (row.node.kind === "child") {
        onAssetSelect?.(row.node.asset.id);
        return;
      }
      // asset
      if (row.hasChildren) toggleExpanded(row.node.id);
      onAssetSelect?.(row.node.asset.id);
    },
    [toggleExpanded, onAssetSelect],
  );

  return (
    <div className="flex h-full w-full flex-col overflow-hidden">
      {/* Search bar */}
      <div className="border-b border-[var(--panel-border)] px-3 py-2">
        <input
          type="text"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          placeholder="Filter assets and zones…"
          className="w-full rounded-md bg-[var(--surface)] px-2.5 py-1.5 text-xs text-[var(--text)] placeholder-[var(--muted)] outline-none ring-0 focus:ring-1 focus:ring-[var(--accent)]/40"
        />
      </div>

      {/* Tree rows */}
      <div className="flex-1 overflow-y-auto py-1">
        {filteredRows.length === 0 && (
          <p className="px-3 py-4 text-center text-xs text-[var(--muted)]">
            {searchQuery ? "No matching items." : "No zones or assets."}
          </p>
        )}
        {filteredRows.map((row) => (
          <TreeRow
            key={row.key}
            row={row}
            expandedIDs={expandedIDs}
            selectedAssetID={selectedAssetID}
            onRowClick={handleRowClick}
          />
        ))}
      </div>
    </div>
  );
}

// ── Single tree row ───────────────────────────────────────────────────────────
interface TreeRowProps {
  row: FlatRow;
  expandedIDs: Set<string>;
  selectedAssetID: string | null;
  onRowClick: (row: FlatRow) => void;
}

const TreeRow = memo(function TreeRow({ row, expandedIDs, selectedAssetID, onRowClick }: TreeRowProps) {
  const { node, depth, hasChildren } = row;
  const isExpanded = expandedIDs.has(row.key);

  const isSelected =
    node.kind !== "zone" && selectedAssetID === node.asset.id;

  const indentPx = depth * 14;

  if (node.kind === "zone") {
    const zoneColor = node.id === "__unsorted__"
      ? "#71717a"
      : (ZONE_TEXT_COLORS[node.zone.color] ?? "#7dd3fc");

    return (
      <button
        onClick={() => onRowClick(row)}
        className={`flex w-full cursor-pointer items-center gap-1.5 px-3 py-1 text-left text-xs hover:bg-[var(--hover)] ${
          isSelected ? "bg-[var(--accent)]/10" : ""
        }`}
        style={{ paddingLeft: `${12 + indentPx}px` }}
      >
        {/* Chevron */}
        <span className="shrink-0 text-[9px] opacity-50" style={{ width: 10 }}>
          {hasChildren ? (isExpanded ? "▼" : "▶") : ""}
        </span>
        {/* Zone icon */}
        <span className="shrink-0 text-[10px]" style={{ color: zoneColor }}>
          ▪
        </span>
        {/* Label */}
        <span className="flex-1 truncate font-medium" style={{ color: zoneColor }}>
          {node.zone.label}
        </span>
        {/* Asset count */}
        {node.assetCount > 0 && (
          <span className="shrink-0 rounded px-1 py-0.5 text-[10px] text-[var(--muted)]">
            {node.assetCount}
          </span>
        )}
      </button>
    );
  }

  if (node.kind === "asset") {
    const status = node.asset.status ?? "unknown";
    const dotColor = STATUS_COLORS[status] ?? STATUS_COLORS.unknown;
    const isChild = depth > 0;

    return (
      <button
        onClick={() => onRowClick(row)}
        className={`flex w-full cursor-pointer items-center gap-1.5 px-3 py-1 text-left text-xs hover:bg-[var(--hover)] ${
          isSelected ? "bg-[var(--accent)]/10" : ""
        }`}
        style={{ paddingLeft: `${12 + indentPx}px` }}
      >
        {/* Chevron */}
        <span className="shrink-0 text-[9px] opacity-50" style={{ width: 10 }}>
          {hasChildren ? (isExpanded ? "▼" : "▶") : ""}
        </span>
        {/* Status dot */}
        <span
          className="shrink-0 h-1.5 w-1.5 rounded-full"
          style={{ background: dotColor }}
        />
        {/* Name */}
        <span
          className={`flex-1 truncate ${isChild ? "text-[var(--muted)]" : "text-[var(--text)]"}`}
        >
          {node.asset.name}
        </span>
        {/* Type badge */}
        <span className="shrink-0 rounded bg-[var(--surface)] px-1 py-0.5 text-[10px] text-[var(--muted)]">
          {node.asset.type}
        </span>
      </button>
    );
  }

  // child
  const status = node.asset.status ?? "unknown";
  const dotColor = STATUS_COLORS[status] ?? STATUS_COLORS.unknown;
  const isSelectedChild = selectedAssetID === node.asset.id;

  return (
    <button
      onClick={() => onRowClick(row)}
      className={`flex w-full cursor-pointer items-center gap-1.5 px-3 py-1 text-left text-xs hover:bg-[var(--hover)] ${
        isSelectedChild ? "bg-[var(--accent)]/10" : ""
      }`}
      style={{ paddingLeft: `${12 + indentPx}px` }}
    >
      <span className="shrink-0" style={{ width: 10 }} />
      <span
        className="shrink-0 h-1.5 w-1.5 rounded-full opacity-60"
        style={{ background: dotColor }}
      />
      <span className="flex-1 truncate text-[var(--muted)]">{node.asset.name}</span>
      <span className="shrink-0 rounded bg-[var(--surface)] px-1 py-0.5 text-[10px] text-[var(--muted)]">
        {node.asset.type}
      </span>
    </button>
  );
});

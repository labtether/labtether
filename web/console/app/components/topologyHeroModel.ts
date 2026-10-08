"use client";

import {
type Edge,
type Node
} from "@xyflow/react";
import {
assetFreshness,
relationshipPriority,
} from "../[locale]/(console)/topology/topologyUtils";
import { type AssetDependency } from "../[locale]/(console)/topology/useAssetDependencies";
import type { Asset,TelemetryOverviewAsset } from "../console/models";

export type StatusTier = "ok" | "warn" | "bad" | "offline";

export type HeroNodeData = {
  label: string;
  status: StatusTier;
  cpuPercent?: number;
  memPercent?: number;
  assetId: string;
  isHub?: boolean;
};

export type HeroEdgeData = {
  alive: boolean;
};

export type HeroEdgeInput = {
  sourceID: string;
  targetID: string;
  relationshipType: string;
  inferred: boolean;
};

export type Point = { x: number; y: number };

export const HERO_MAX_NODES = 24;

export function freshnessToTier(asset: Asset): StatusTier {
  const f = assetFreshness(asset);
  if (f === "online") return "ok";
  if (f === "unresponsive") return "warn";
  if (f === "offline") return "bad";
  return "offline";
}

export const STATUS_COLORS: Record<StatusTier, { ring: string; glow: string; glowVar: string; rgb: string }> = {
  ok: {
    ring: "var(--ok)",
    glow: "var(--ok-glow)",
    glowVar: "0 0 14px 3px var(--ok-glow), 0 0 4px 1px var(--ok-glow)",
    rgb: "var(--ok)",
  },
  warn: {
    ring: "var(--warn)",
    glow: "var(--warn-glow)",
    glowVar: "0 0 14px 3px var(--warn-glow), 0 0 4px 1px var(--warn-glow)",
    rgb: "var(--warn)",
  },
  bad: {
    ring: "var(--bad)",
    glow: "var(--bad-glow)",
    glowVar: "0 0 14px 3px var(--bad-glow), 0 0 4px 1px var(--bad-glow)",
    rgb: "var(--bad)",
  },
  offline: {
    ring: "#71717a",
    glow: "transparent",
    glowVar: "none",
    rgb: "#71717a",
  },
};

export function layoutNodes(assets: Asset[]): { x: number; y: number }[] {
  const count = assets.length;
  if (count === 0) return [];
  if (count === 1) return [{ x: 200, y: 110 }];

  // Arrange nodes in an ellipse
  const cx = 200;
  const cy = 110;
  const rx = 160;
  const ry = 80;
  const positions: { x: number; y: number }[] = [];

  for (let i = 0; i < count; i++) {
    const angle = (2 * Math.PI * i) / count - Math.PI / 2;
    positions.push({
      x: cx + rx * Math.cos(angle),
      y: cy + ry * Math.sin(angle),
    });
  }

  return positions;
}

export function pointDistance(left: Point, right: Point): number {
  return Math.hypot(right.x - left.x, right.y - left.y);
}

export function pickDirection(from: Point, to: Point): "left" | "right" | "top" | "bottom" {
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  if (Math.abs(dx) >= Math.abs(dy)) {
    return dx >= 0 ? "right" : "left";
  }
  return dy >= 0 ? "bottom" : "top";
}

export function oppositeDirection(direction: "left" | "right" | "top" | "bottom"): "left" | "right" | "top" | "bottom" {
  if (direction === "left") return "right";
  if (direction === "right") return "left";
  if (direction === "top") return "bottom";
  return "top";
}

export function resolveEdgeHandles(sourcePos: Point, targetPos: Point): { sourceHandle: string; targetHandle: string } {
  const sourceDirection = pickDirection(sourcePos, targetPos);
  const targetDirection = oppositeDirection(sourceDirection);
  return {
    sourceHandle: `${sourceDirection}-out`,
    targetHandle: `${targetDirection}-in`,
  };
}

export function buildDependencyEdgeInputs(
  selectedByID: Map<string, Asset>,
  positionsByID: Map<string, Point>,
  dependencies: AssetDependency[],
): HeroEdgeInput[] {
  const ranked: Array<HeroEdgeInput & { priority: number; distance: number }> = [];
  const seen = new Set<string>();

  for (const dependency of dependencies) {
    const sourceID = dependency.source_asset_id;
    const targetID = dependency.target_asset_id;
    if (sourceID === targetID) continue;
    if (!selectedByID.has(sourceID) || !selectedByID.has(targetID)) continue;

    const relationshipType = dependency.relationship_type.trim().toLowerCase();
    if (!relationshipType) continue;

    const dedupeKey = `${sourceID}->${targetID}:${relationshipType}`;
    if (seen.has(dedupeKey)) continue;
    seen.add(dedupeKey);

    const sourcePos = positionsByID.get(sourceID);
    const targetPos = positionsByID.get(targetID);
    const distance = sourcePos && targetPos ? pointDistance(sourcePos, targetPos) : Number.MAX_SAFE_INTEGER;

    ranked.push({
      sourceID,
      targetID,
      relationshipType,
      inferred: false,
      priority: relationshipPriority(relationshipType, false),
      distance,
    });
  }

  if (ranked.length === 0) return [];

  const maxEdges = Math.max(2, Math.min(18, selectedByID.size + 4));
  ranked.sort((left, right) => left.priority - right.priority || left.distance - right.distance);
  return ranked.slice(0, maxEdges).map(({ priority: _priority, distance: _distance, ...edge }) => edge);
}

export function buildMstEdgeInputs(selected: Asset[], positionsByID: Map<string, Point>): HeroEdgeInput[] {
  if (selected.length < 2) return [];

  const ids = selected.map((asset) => asset.id);
  const candidates: Array<{ leftID: string; rightID: string; distance: number }> = [];
  for (let left = 0; left < ids.length - 1; left += 1) {
    for (let right = left + 1; right < ids.length; right += 1) {
      const leftID = ids[left];
      const rightID = ids[right];
      const leftPos = positionsByID.get(leftID);
      const rightPos = positionsByID.get(rightID);
      if (!leftPos || !rightPos) continue;
      candidates.push({
        leftID,
        rightID,
        distance: pointDistance(leftPos, rightPos),
      });
    }
  }
  candidates.sort((left, right) => left.distance - right.distance);

  const parent = new Map(ids.map((id) => [id, id]));
  const rank = new Map(ids.map((id) => [id, 0]));

  const find = (id: string): string => {
    const nodeParent = parent.get(id) ?? id;
    if (nodeParent === id) return id;
    const root = find(nodeParent);
    parent.set(id, root);
    return root;
  };

  const union = (leftID: string, rightID: string): boolean => {
    const leftRoot = find(leftID);
    const rightRoot = find(rightID);
    if (leftRoot === rightRoot) return false;

    const leftRank = rank.get(leftRoot) ?? 0;
    const rightRank = rank.get(rightRoot) ?? 0;
    if (leftRank < rightRank) {
      parent.set(leftRoot, rightRoot);
      return true;
    }
    if (leftRank > rightRank) {
      parent.set(rightRoot, leftRoot);
      return true;
    }
    parent.set(rightRoot, leftRoot);
    rank.set(leftRoot, leftRank + 1);
    return true;
  };

  const edges: HeroEdgeInput[] = [];
  for (const candidate of candidates) {
    if (edges.length >= selected.length - 1) break;
    if (!union(candidate.leftID, candidate.rightID)) continue;

    const leftPos = positionsByID.get(candidate.leftID);
    const rightPos = positionsByID.get(candidate.rightID);
    if (!leftPos || !rightPos) continue;

    const useForwardDirection = leftPos.x < rightPos.x || (leftPos.x === rightPos.x && leftPos.y <= rightPos.y);
    edges.push({
      sourceID: useForwardDirection ? candidate.leftID : candidate.rightID,
      targetID: useForwardDirection ? candidate.rightID : candidate.leftID,
      relationshipType: "connected_to",
      inferred: true,
    });
  }

  return edges;
}

export function buildHeroGraph(
  visibleAssets: Asset[],
  telemetry: TelemetryOverviewAsset[],
  dependencies: AssetDependency[],
): { nodes: Node[]; edges: Edge[] } {
  if (visibleAssets.length === 0) {
    return { nodes: [], edges: [] };
  }

  const sorted = [...visibleAssets].sort((a, b) => a.name.localeCompare(b.name));
  const selected = sorted.slice(0, HERO_MAX_NODES);
  const selectedByID = new Map<string, Asset>(selected.map((asset) => [asset.id, asset]));

  // Build telemetry lookup
  const telemetryByID = new Map<string, TelemetryOverviewAsset>();
  for (const t of telemetry) {
    telemetryByID.set(t.asset_id, t);
  }

  // Layout
  const positions = layoutNodes(selected);
  const positionsByID = new Map<string, Point>();
  for (let index = 0; index < selected.length; index += 1) {
    const asset = selected[index];
    const point = positions[index];
    if (!point) continue;
    positionsByID.set(asset.id, point);
  }

  // Determine hub nodes — nodes that have the most incoming dependencies
  const incomingCount = new Map<string, number>();
  for (const dep of dependencies) {
    if (selectedByID.has(dep.target_asset_id)) {
      incomingCount.set(dep.target_asset_id, (incomingCount.get(dep.target_asset_id) ?? 0) + 1);
    }
  }
  const maxIncoming = Math.max(0, ...incomingCount.values());
  const hubThreshold = Math.max(2, Math.floor(maxIncoming * 0.7));

  const nodes: Node[] = selected.map((asset, i) => {
    const tel = telemetryByID.get(asset.id);
    const position = positions[i] ?? { x: 200, y: 110 };
    return {
      id: asset.id,
      type: "heroNode",
      position,
      draggable: false,
      connectable: false,
      selectable: false,
      data: {
        label: asset.name,
        status: freshnessToTier(asset),
        cpuPercent: tel?.metrics.cpu_used_percent,
        memPercent: tel?.metrics.memory_used_percent,
        assetId: asset.id,
        isHub: (incomingCount.get(asset.id) ?? 0) >= hubThreshold,
      },
    };
  });

  const dependencyEdges = buildDependencyEdgeInputs(selectedByID, positionsByID, dependencies);
  const edgeInputs = dependencyEdges.length > 0
    ? dependencyEdges
    : buildMstEdgeInputs(selected, positionsByID);

  const edges: Edge[] = [];
  for (let index = 0; index < edgeInputs.length; index += 1) {
    const edgeInput = edgeInputs[index];
    const source = selectedByID.get(edgeInput.sourceID);
    const target = selectedByID.get(edgeInput.targetID);
    const sourcePos = positionsByID.get(edgeInput.sourceID);
    const targetPos = positionsByID.get(edgeInput.targetID);
    if (!source || !target || !sourcePos || !targetPos) continue;

    const sourceTier = freshnessToTier(source);
    const targetTier = freshnessToTier(target);
    const alive = sourceTier !== "bad" && sourceTier !== "offline"
      && targetTier !== "bad" && targetTier !== "offline";
    const { sourceHandle, targetHandle } = resolveEdgeHandles(sourcePos, targetPos);
    const sc_source = STATUS_COLORS[sourceTier];

    edges.push({
      id: `hero-${edgeInput.sourceID}-${edgeInput.targetID}-${edgeInput.relationshipType}-${index}`,
      source: edgeInput.sourceID,
      target: edgeInput.targetID,
      sourceHandle,
      targetHandle,
      type: "straight",
      data: { alive },
      style: {
        stroke: alive ? sc_source.ring : "#3f3f46",
        strokeWidth: 1.5,
        strokeDasharray: "6 4",
        opacity: alive ? 0.45 : 0.2,
      },
      animated: alive,
    });
  }

  return { nodes, edges };
}

import type { Asset } from "../../../console/models";
import { childParentKey, hostParentKey, isInfraHost } from "../../../console/taxonomy";
import { inferRelationshipType } from "./topologySmartDefaults";
import type { RelationshipType, TopologyConnection } from "./topologyCanvasTypes";

export type DisplayConnection = {
  id: string;
  source_asset_id: string;
  target_asset_id: string;
  relationship: RelationshipType;
  inferred: boolean;
};

function inferParentRelationship(child: Asset, parent: Asset): RelationshipType {
  if (child.source === "docker" && parent.source === "docker") {
    return "hosted_on";
  }
  if (child.type === "docker-container" && parent.type === "container-host") {
    return "hosted_on";
  }
  return inferRelationshipType(child.type, parent.type);
}

export function buildDisplayConnections(
  assets: Asset[],
  connections: TopologyConnection[],
): DisplayConnection[] {
  const assetByID = new Map<string, Asset>();
  for (const asset of assets) {
    assetByID.set(asset.id, asset);
  }

  const displayConnections: DisplayConnection[] = connections.map((connection) => ({
    id: connection.id,
    source_asset_id: connection.source_asset_id,
    target_asset_id: connection.target_asset_id,
    relationship: connection.relationship,
    inferred: false,
  }));

  const seenEdges = new Set(displayConnections.map((connection) => `${connection.source_asset_id}->${connection.target_asset_id}`));
  const hostsByParentKey = new Map<string, Asset>();
  for (const asset of assets) {
    if (!isInfraHost(asset)) {
      continue;
    }
    const parentKey = hostParentKey(asset);
    if (!parentKey || hostsByParentKey.has(parentKey)) {
      continue;
    }
    hostsByParentKey.set(parentKey, asset);
  }

  for (const asset of assets) {
    if (isInfraHost(asset)) {
      continue;
    }
    const parentKey = childParentKey(asset);
    if (!parentKey) {
      continue;
    }
    const parent = hostsByParentKey.get(parentKey);
    if (!parent || parent.id === asset.id) {
      continue;
    }
    const edgeKey = `${asset.id}->${parent.id}`;
    if (seenEdges.has(edgeKey)) {
      continue;
    }
    seenEdges.add(edgeKey);
    displayConnections.push({
      id: `inferred:${edgeKey}`,
      source_asset_id: asset.id,
      target_asset_id: parent.id,
      relationship: inferParentRelationship(asset, parent),
      inferred: true,
    });
  }

  return displayConnections;
}

export function buildChildAssetsByParent(
  assets: Asset[],
  connections: TopologyConnection[],
): Map<string, Asset[]> {
  const assetByID = new Map<string, Asset>();
  for (const asset of assets) {
    assetByID.set(asset.id, asset);
  }

  const childrenByParent = new Map<string, Asset[]>();
  for (const connection of buildDisplayConnections(assets, connections)) {
    if (connection.relationship !== "hosted_on" && connection.relationship !== "runs_on") {
      continue;
    }
    const child = assetByID.get(connection.source_asset_id);
    if (!child) {
      continue;
    }
    const siblings = childrenByParent.get(connection.target_asset_id) ?? [];
    if (!siblings.some((candidate) => candidate.id === child.id)) {
      siblings.push(child);
      childrenByParent.set(connection.target_asset_id, siblings);
    }
  }

  return childrenByParent;
}

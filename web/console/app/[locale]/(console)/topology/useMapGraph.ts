"use client";

import { useMemo } from "react";
import { useFastStatus } from "../../../contexts/StatusContext";
import { isInfraHost } from "../../../console/taxonomy";
import { buildDeviceCards } from "../nodes/deviceCards";
import { buildMapGraph } from "./mapModel";
import { buildHierarchy } from "./topologyHierarchy";
import { useAssetDependencies, type AssetDependency } from "./useAssetDependencies";
import { useTopologyData } from "./useTopologyData";

const NO_IDS: string[] = [];

/**
 * The Map's data: the hub's devices (status), the stored topology (zones, connections, unsorted) and, on the Map page,
 * the relationship edges that place guests under hosts. edges: false (the Dashboard) skips the edges request; guests
 * then come from the topology's own connections and the device taxonomy.
 */
export function useMapGraph({ edges = true }: { edges?: boolean } = {}) {
  const status = useFastStatus();
  const data = useTopologyData();
  const { topology } = data;
  const assets = useMemo(() => status?.assets ?? [], [status?.assets]);
  const cards = useMemo(
    () => [...buildDeviceCards(assets, status?.telemetryOverview ?? []).cardsByGroupID.values()].flat(),
    [assets, status?.telemetryOverview],
  );
  const assetIDs = useMemo(() => (edges ? assets.map((a) => a.id).sort() : NO_IDS), [edges, assets]);
  const { dependencies } = useAssetDependencies(assetIDs);

  const hierarchy = useMemo(() => {
    const merged: AssetDependency[] = [...dependencies];
    const seen = new Set(merged.map((d) => d.id));
    for (const c of topology?.connections ?? []) {
      if (seen.has(c.id)) continue;
      merged.push({
        id: c.id, source_asset_id: c.source_asset_id, target_asset_id: c.target_asset_id,
        relationship_type: c.relationship, origin: c.origin === "user" ? "manual" : "auto",
      });
    }
    return buildHierarchy(assets, merged);
  }, [assets, dependencies, topology?.connections]);

  const graph = useMemo(
    () => buildMapGraph({ assets, cards, topology, parentOf: hierarchy.parentOf, isHost: isInfraHost }),
    [assets, cards, topology, hierarchy.parentOf],
  );

  return { graph, assets, hierarchy, statusLoaded: !!status, ...data };
}

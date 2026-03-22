"use client";

import { useMemo } from "react";
import type { Asset } from "../../../../console/models";
import { isInfraHost } from "../../../../console/taxonomy";
import {
  collectIdentityFromAsset,
  normalizeAssetName,
  type IdentitySnapshot,
} from "./clusterTopologyUtils";
import type {
  ClusterStatusEntry,
  HAResource,
} from "./clusterTopologyTypes";

type UseClusterTopologyDerivedDataArgs = {
  clusterStatus: ClusterStatusEntry[];
  haResources?: HAResource[];
  assets: Asset[];
};

export function useClusterTopologyDerivedData({
  clusterStatus,
  haResources,
  assets,
}: UseClusterTopologyDerivedDataArgs) {
  const clusterEntry = useMemo(
    () => clusterStatus.find((entry) => entry.type === "cluster"),
    [clusterStatus],
  );

  const nodeEntries = useMemo(
    () => clusterStatus.filter((entry) => entry.type === "node"),
    [clusterStatus],
  );

  const haByNode = useMemo(() => {
    const map = new Map<string, HAResource[]>();
    if (!haResources) {
      return map;
    }
    for (const resource of haResources) {
      const nodeName = resource.node ?? "unassigned";
      const nodeResources = map.get(nodeName) ?? [];
      nodeResources.push(resource);
      map.set(nodeName, nodeResources);
    }
    return map;
  }, [haResources]);

  const guestsByNode = useMemo(() => {
    const map = new Map<string, Asset[]>();
    for (const asset of assets) {
      if (asset.source !== "proxmox") {
        continue;
      }
      if (asset.type !== "vm" && asset.type !== "container") {
        continue;
      }
      const nodeName = (asset.metadata?.node ?? "").trim();
      if (!nodeName) {
        continue;
      }
      const nodeGuests = map.get(nodeName) ?? [];
      nodeGuests.push(asset);
      map.set(nodeName, nodeGuests);
    }
    for (const [nodeName, nodeGuests] of map.entries()) {
      map.set(nodeName, [...nodeGuests].sort((a, b) => a.name.localeCompare(b.name)));
    }
    return map;
  }, [assets]);

  const apiHosts = useMemo(() => {
    return assets
      .filter((asset) => asset.source !== "proxmox" && isInfraHost(asset))
      .sort((a, b) => a.name.localeCompare(b.name));
  }, [assets]);

  const apiHostsByID = useMemo(() => {
    const map = new Map<string, Asset>();
    for (const host of apiHosts) {
      map.set(host.id, host);
    }
    return map;
  }, [apiHosts]);

  const apiHostsByName = useMemo(() => {
    const map = new Map<string, Asset[]>();
    for (const host of apiHosts) {
      const key = normalizeAssetName(host.name);
      if (!key) {
        continue;
      }
      const matchingHosts = map.get(key) ?? [];
      matchingHosts.push(host);
      map.set(key, matchingHosts);
    }
    return map;
  }, [apiHosts]);

  const hostIdentityByID = useMemo(() => {
    const map = new Map<string, IdentitySnapshot>();
    for (const host of apiHosts) {
      map.set(host.id, collectIdentityFromAsset(host));
    }
    return map;
  }, [apiHosts]);

  const guestAssets = useMemo(() => {
    const nodeNames = new Set(
      nodeEntries
        .map((entry) => (entry.name ?? "").trim())
        .filter((name) => name.length > 0),
    );
    return assets.filter((asset) => {
      if (asset.source !== "proxmox") {
        return false;
      }
      if (asset.type !== "vm" && asset.type !== "container") {
        return false;
      }
      const nodeName = (asset.metadata?.node ?? "").trim();
      if (!nodeName) {
        return false;
      }
      return nodeNames.has(nodeName);
    });
  }, [assets, nodeEntries]);

  const guestsByID = useMemo(() => {
    const map = new Map<string, Asset>();
    for (const guest of guestAssets) {
      map.set(guest.id, guest);
    }
    return map;
  }, [guestAssets]);

  const proxmoxNodeAssetIDByName = useMemo(() => {
    const map = new Map<string, string>();
    for (const asset of assets) {
      if (asset.source !== "proxmox") {
        continue;
      }
      if (!isInfraHost(asset) && asset.type !== "connector-cluster") {
        continue;
      }
      const candidateNames = [asset.name, asset.metadata?.node ?? ""];
      for (const candidateName of candidateNames) {
        const key = normalizeAssetName(candidateName);
        if (!key || map.has(key)) {
          continue;
        }
        map.set(key, asset.id);
      }
    }
    return map;
  }, [assets]);

  return {
    clusterEntry,
    nodeEntries,
    haByNode,
    guestsByNode,
    apiHosts,
    apiHostsByID,
    apiHostsByName,
    hostIdentityByID,
    guestAssets,
    guestsByID,
    proxmoxNodeAssetIDByName,
  };
}

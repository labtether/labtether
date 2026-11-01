"use client";

import { useMemo } from "react";
import { useFastStatus } from "../../../contexts/StatusContext";
import { useConnectedAgents } from "../../../hooks/useConnectedAgents";
import type { Asset } from "../../../console/models";
import { useFileConnections } from "./useFileConnections";

// What Files can open: devices whose agent is connected, the devices it can't open (and why), and saved connections.

export type Unavailable = { asset: Asset; reason: "offline" | "noAgent" };

const isUp = (status: string) => ["online", "ok", "up", "healthy"].includes(status.toLowerCase());

export function useFileSources() {
  const status = useFastStatus();
  const { connectedAgentIds } = useConnectedAgents();
  const { connections, loading, refetch } = useFileConnections();

  const { ready, unavailable } = useMemo(() => {
    const all = [...(status?.assets ?? [])].sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }));
    const r: Asset[] = [];
    const u: Unavailable[] = [];
    for (const asset of all) {
      if (connectedAgentIds.has(asset.id)) r.push(asset);
      else u.push({ asset, reason: isUp(asset.status ?? "") ? "noAgent" : "offline" });
    }
    return { ready: r, unavailable: u };
  }, [status, connectedAgentIds]);

  return { ready, unavailable, connections, connectionsLoading: loading, refetchConnections: refetch, connectedAgentIds };
}

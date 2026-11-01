"use client";

import { useCallback, useMemo, useState } from "react";
import { useStatusAssetNameMap } from "../../contexts/StatusContext";
import { usePollingTask } from "../../hooks/usePollingTask";
import {
  fetchDockerContainers,
  fetchDockerHosts,
  fetchPortainerContainers,
  fetchPortainerEndpoints,
  type DockerContainer,
  type DockerHostSummary,
} from "../../../lib/docker";
import { toHost, toRow, type CtHost, type CtRow } from "./containerModel";

type Loaded = { host: DockerHostSummary; containers: DockerContainer[] };

/**
 * Every Docker host (agents) and Portainer endpoint with its containers, polled every 15 s. A host that does not
 * answer is counted in `failed` and left out of the list.
 */
export function useContainerFleet() {
  const [loaded, setLoaded] = useState<Loaded[]>([]);
  const [failed, setFailed] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const names = useStatusAssetNameMap();

  const task = useCallback(async (signal: AbortSignal) => {
    try {
      const [docker, portainer] = await Promise.all([fetchDockerHosts(signal), fetchPortainerEndpoints(signal)]);
      if (signal.aborted) return;
      const all = [...docker.map((h) => ({ ...h, source: "docker" as const })), ...portainer];
      const results = await Promise.allSettled(all.map(async (host) => ({
        host,
        containers: host.source === "portainer" ? await fetchPortainerContainers(host.agent_id, signal) : await fetchDockerContainers(host.agent_id, signal),
      })));
      if (signal.aborted) return;
      setLoaded(results.flatMap((r) => (r.status === "fulfilled" ? [r.value] : [])));
      setFailed(results.filter((r) => r.status === "rejected").length);
      setError(null);
    } catch (err) {
      if (signal.aborted) return;
      setError(err instanceof Error ? err.message : "");
    } finally {
      if (!signal.aborted) setLoading(false);
    }
  }, []);

  const reload = usePollingTask(task, 15_000);

  const { hosts, rows } = useMemo(() => {
    const hostList: CtHost[] = [];
    const rowList: CtRow[] = [];
    for (const { host, containers } of loaded) {
      const h = toHost(host, names);
      hostList.push(h);
      for (const c of containers) rowList.push(toRow(h, c));
    }
    hostList.sort((a, b) => a.label.localeCompare(b.label));
    return { hosts: hostList, rows: rowList };
  }, [loaded, names]);

  return { hosts, rows, loading, error, failed, reload };
}

"use client";

import { useCallback, useState } from "react";
import { usePollingTask } from "../../hooks/usePollingTask";
import { fetchDockerContainers, fetchDockerStacks, type DockerContainer, type DockerStack } from "../../../lib/docker";
import { dockerHostAssetId, hostKey, nodeHref, type CtHost } from "./containerModel";

/** One Docker host (agent) as a list host. */
export const agentHost = (hostId: string): CtHost => ({
  key: hostKey(hostId),
  label: hostId,
  source: "docker",
  href: nodeHref(dockerHostAssetId(hostId)),
  canDeploy: true,
});

/**
 * A Docker host's containers (and its compose stacks when withStacks), polled every intervalMs. error is the hub's
 * message, kept until the next good answer; data from the last good answer stays on screen.
 */
export function useHostDocker(hostId: string, { withStacks = false, intervalMs = 10_000 } = {}) {
  const [containers, setContainers] = useState<DockerContainer[]>([]);
  const [stacks, setStacks] = useState<DockerStack[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const task = useCallback(async (signal: AbortSignal) => {
    if (!hostId) { setLoading(false); return; }
    try {
      const [list, stackList] = await Promise.all([
        fetchDockerContainers(hostId, signal),
        withStacks ? fetchDockerStacks(hostId, signal) : Promise.resolve([] as DockerStack[]),
      ]);
      if (signal.aborted) return;
      setContainers(list);
      setStacks(stackList);
      setError(null);
    } catch (err) {
      if (signal.aborted) return;
      setError(err instanceof Error && err.message ? err.message : "error");
    } finally {
      if (!signal.aborted) setLoading(false);
    }
  }, [hostId, withStacks]);

  const reload = usePollingTask(task, intervalMs);
  return { containers, stacks, loading, error, reload };
}

"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "../../../../../i18n/navigation";
import { Card } from "../../../../components/ui/Card";
import { Badge } from "../../../../components/ui/Badge";
import type { DockerContainer, DockerStack } from "../../../../../lib/docker";
import { fetchDockerContainers, fetchDockerStacks } from "../../../../../lib/docker";

type Props = {
  hostId: string;
  stackName: string;
};

function normalizeHostID(hostId: string): string {
  return hostId
    .trim()
    .toLowerCase()
    .replaceAll(" ", "-")
    .replaceAll(".", "-")
    .replace(/^docker-host-/, "")
    .replace(/^docker-/, "");
}

function containerAssetID(hostId: string, containerID: string): string {
  const shortID = containerID.length > 12 ? containerID.slice(0, 12) : containerID;
  return `docker-ct-${normalizeHostID(hostId)}-${shortID}`;
}

export function DockerStackContainersTab({ hostId, stackName }: Props) {
  const [stack, setStack] = useState<DockerStack | null>(null);
  const [containers, setContainers] = useState<DockerContainer[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [stacks, hostContainers] = await Promise.all([
        fetchDockerStacks(hostId),
        fetchDockerContainers(hostId),
      ]);
      const matchedStack = stacks.find((entry) => entry.name.toLowerCase() === stackName.toLowerCase()) ?? null;
      setStack(matchedStack);
      setContainers(hostContainers);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load stack containers");
    } finally {
      setLoading(false);
    }
  }, [hostId, stackName]);

  useEffect(() => {
    void load();
    const timer = setInterval(() => void load(), 10_000);
    return () => clearInterval(timer);
  }, [load]);

  const stackContainers = useMemo(() => {
    if (!stack) return [];
    const linked = new Set(stack.container_ids.map((value) => value.toLowerCase()));
    return containers.filter((container) => {
      if ((container.stack_name ?? "").toLowerCase() === stack.name.toLowerCase()) return true;
      if (linked.has(container.name.toLowerCase())) return true;
      if (linked.has(container.id.toLowerCase())) return true;
      const shortID = container.id.length > 12 ? container.id.slice(0, 12) : container.id;
      return linked.has(shortID.toLowerCase());
    });
  }, [containers, stack]);

  function badgeStatus(state: string): "ok" | "pending" | "bad" {
    if (state === "running") return "ok";
    if (state === "paused") return "pending";
    return "bad";
  }

  return (
    <Card className="mb-4">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-medium text-[var(--text)]">Stack Containers</h2>
        <span className="text-xs text-[var(--muted)]">{stackContainers.length} linked</span>
      </div>

      {loading ? (
        <p className="text-sm text-[var(--muted)]">Loading containers...</p>
      ) : stackContainers.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-2 text-left font-medium text-[var(--muted)]">Container</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Image</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">State</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--line)]">
              {stackContainers.map((container) => (
                <tr key={container.id} className="hover:bg-[var(--hover)]">
                  <td className="py-2 font-medium text-[var(--text)]">
                    <Link
                      href={`/nodes/${encodeURIComponent(containerAssetID(hostId, container.id))}`}
                      className="text-[var(--accent)] hover:underline"
                    >
                      {container.name}
                    </Link>
                  </td>
                  <td className="max-w-56 truncate py-2 text-[var(--muted)]">{container.image}</td>
                  <td className="py-2">
                    <Badge status={badgeStatus(container.state)} size="sm" />
                  </td>
                  <td className="py-2 text-[var(--muted)]">{container.status || "--"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center gap-2 py-12">
          <p className="text-sm font-medium text-[var(--text)]">No linked containers</p>
          <p className="text-xs text-[var(--muted)]">No containers currently matched to this stack.</p>
        </div>
      )}

      {error ? <p className="mt-3 text-xs text-[var(--bad)]">{error}</p> : null}
    </Card>
  );
}


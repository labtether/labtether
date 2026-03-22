"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "../../../../../i18n/navigation";
import { Card } from "../../../../components/ui/Card";
import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { SegmentedTabs } from "../../../../components/ui/SegmentedTabs";
import { useDocumentVisibility } from "../../../../hooks/useDocumentVisibility";
import type { DockerContainer } from "../../../../../lib/docker";
import { fetchDockerContainers, executeContainerAction } from "../../../../../lib/docker";

type Props = {
  hostId: string;
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

function containerAssetId(hostId: string, containerID: string): string {
  const shortID = containerID.length > 12 ? containerID.slice(0, 12) : containerID;
  return `docker-ct-${normalizeHostID(hostId)}-${shortID}`;
}

export function DockerContainersTab({ hostId }: Props) {
  const [containers, setContainers] = useState<DockerContainer[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<"all" | "running" | "stopped">("all");
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const isDocumentVisible = useDocumentVisibility();
  const loadInFlightRef = useRef(false);

  const load = useCallback(async () => {
    if (loadInFlightRef.current) {
      return;
    }
    loadInFlightRef.current = true;
    try {
      const data = await fetchDockerContainers(hostId);
      setContainers(data);
      setActionError(null);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "failed to load containers");
    } finally {
      loadInFlightRef.current = false;
      setLoading(false);
    }
  }, [hostId]);

  useEffect(() => {
    if (!isDocumentVisible) {
      return;
    }
    void load();
    const interval = setInterval(() => void load(), 10_000);
    return () => clearInterval(interval);
  }, [isDocumentVisible, load]);

  const runAction = async (containerID: string, action: string) => {
    setActionInFlight(containerID);
    setActionError(null);
    try {
      await executeContainerAction(containerID, action);
      setTimeout(() => void load(), 900);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "container action failed");
    } finally {
      setActionInFlight(null);
    }
  };

  const filtered = containers.filter((container) => {
    if (filter === "running") return container.state === "running";
    if (filter === "stopped") return container.state !== "running";
    return true;
  });

  const runningCount = containers.filter((container) => container.state === "running").length;
  const stoppedCount = containers.length - runningCount;

  const hostNodeID = useMemo(() => `docker-host-${normalizeHostID(hostId)}`, [hostId]);
  const newContainerHref = `/nodes/${encodeURIComponent(hostNodeID)}/new-container`;
  const newComposeHref = `/nodes/${encodeURIComponent(hostNodeID)}/new-compose`;

  function stateBadge(state: string): "ok" | "pending" | "bad" {
    if (state === "running") return "ok";
    if (state === "paused") return "pending";
    return "bad";
  }

  return (
    <Card className="mb-4">
      <div className="mb-4 rounded-xl border border-[var(--line)] bg-[linear-gradient(135deg,var(--panel)_0%%,color-mix(in_oklab,var(--accent)_10%%,var(--panel))_100%%)] p-4">
        <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-wider text-[var(--muted)]">Containers</p>
            <p className="mt-1 text-sm text-[var(--text)]">
              Manage lifecycle here, and use dedicated pages for creating containers or deploying compose apps.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Link
              href={newContainerHref}
              className="inline-flex items-center justify-center rounded-lg bg-[var(--control-bg-active)] px-3 py-1.5 text-xs font-medium text-[var(--control-fg-active)] transition-opacity hover:opacity-90"
            >
              New Container
            </Link>
            <Link
              href={newComposeHref}
              className="inline-flex items-center justify-center rounded-lg border border-[var(--control-border)] px-3 py-1.5 text-xs font-medium text-[var(--control-fg)] transition-colors hover:bg-[var(--control-bg-hover)]"
            >
              New Compose
            </Link>
          </div>
        </div>
      </div>

      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-medium text-[var(--text)]">Container Inventory</h2>
        <SegmentedTabs
          size="sm"
          value={filter}
          options={[
            { id: "all", label: `All (${containers.length})` },
            { id: "running", label: `Running (${runningCount})` },
            { id: "stopped", label: `Stopped (${stoppedCount})` },
          ]}
          onChange={setFilter}
        />
      </div>

      {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}

      {loading ? (
        <p className="text-sm text-[var(--muted)]">Loading containers...</p>
      ) : filtered.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-2 text-left font-medium text-[var(--muted)]">Name</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Image</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">State</th>
                <th className="py-2 text-right font-medium text-[var(--muted)]">CPU</th>
                <th className="py-2 text-right font-medium text-[var(--muted)]">Memory</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Stack</th>
                <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--line)]">
              {filtered.map((container) => {
                const assetID = containerAssetId(hostId, container.id);
                return (
                  <tr key={container.id} className="hover:bg-[var(--hover)]">
                    <td className="py-2 font-medium text-[var(--text)]">
                      <Link
                        href={`/nodes/${encodeURIComponent(assetID)}`}
                        className="text-[var(--accent)] hover:underline"
                      >
                        {container.name}
                      </Link>
                    </td>
                    <td className="max-w-56 truncate py-2 text-[var(--muted)]">{container.image}</td>
                    <td className="py-2">
                      <Badge status={stateBadge(container.state)} size="sm" />
                    </td>
                    <td className="py-2 text-right text-[var(--muted)]">
                      {container.cpu_percent != null ? `${container.cpu_percent.toFixed(1)}%` : "--"}
                    </td>
                    <td className="py-2 text-right text-[var(--muted)]">
                      {container.memory_percent != null ? `${container.memory_percent.toFixed(1)}%` : "--"}
                    </td>
                    <td className="py-2 text-[var(--muted)]">{container.stack_name || "--"}</td>
                    <td className="py-2 text-right">
                      <div className="flex items-center justify-end gap-1">
                        {container.state === "running" ? (
                          <>
                            <Button
                              size="sm"
                              disabled={actionInFlight === assetID}
                              onClick={() => void runAction(assetID, "container.restart")}
                            >
                              Restart
                            </Button>
                            <Button
                              size="sm"
                              variant="danger"
                              disabled={actionInFlight === assetID}
                              onClick={() => void runAction(assetID, "container.stop")}
                            >
                              Stop
                            </Button>
                          </>
                        ) : (
                          <Button
                            size="sm"
                            disabled={actionInFlight === assetID}
                            onClick={() => void runAction(assetID, "container.start")}
                          >
                            Start
                          </Button>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center gap-2 py-12">
          <p className="text-sm font-medium text-[var(--text)]">No containers</p>
          <p className="text-xs text-[var(--muted)]">No containers found on this Docker host.</p>
          <div className="mt-2 flex items-center gap-2">
            <Link
              href={newContainerHref}
              className="inline-flex items-center justify-center rounded-lg bg-[var(--control-bg-active)] px-3 py-1.5 text-xs font-medium text-[var(--control-fg-active)] transition-opacity hover:opacity-90"
            >
              Create First Container
            </Link>
            <Link
              href={newComposeHref}
              className="inline-flex items-center justify-center rounded-lg border border-[var(--control-border)] px-3 py-1.5 text-xs font-medium text-[var(--control-fg)] transition-colors hover:bg-[var(--control-bg-hover)]"
            >
              Deploy Compose
            </Link>
          </div>
        </div>
      )}
    </Card>
  );
}

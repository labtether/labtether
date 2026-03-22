"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "../../../../../i18n/navigation";
import { Card } from "../../../../components/ui/Card";
import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import type { DockerStack } from "../../../../../lib/docker";
import { fetchDockerStacks, executeStackAction } from "../../../../../lib/docker";

type Props = { hostId: string };

function normalizeHostID(hostId: string): string {
  return hostId
    .trim()
    .toLowerCase()
    .replaceAll(" ", "-")
    .replaceAll(".", "-")
    .replace(/^docker-host-/, "")
    .replace(/^docker-/, "");
}

function stackAssetId(hostId: string, stackName: string): string {
  const normalizedStack = stackName.trim().toLowerCase().replaceAll(" ", "-").replaceAll(".", "-");
  return `docker-stack-${normalizeHostID(hostId)}-${normalizedStack}`;
}

export function DockerStacksTab({ hostId }: Props) {
  const [stacks, setStacks] = useState<DockerStack[]>([]);
  const [loading, setLoading] = useState(true);
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const data = await fetchDockerStacks(hostId);
      setStacks(data);
      setActionError(null);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "failed to load stacks");
    } finally {
      setLoading(false);
    }
  }, [hostId]);

  useEffect(() => {
    void load();
    const interval = setInterval(() => void load(), 15_000);
    return () => clearInterval(interval);
  }, [load]);

  const runAction = async (targetStackID: string, action: string) => {
    setActionInFlight(targetStackID);
    setActionError(null);
    try {
      await executeStackAction(targetStackID, action);
      setTimeout(() => void load(), 1400);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "stack action failed");
    } finally {
      setActionInFlight(null);
    }
  };

  const hostNodeID = useMemo(() => `docker-host-${normalizeHostID(hostId)}`, [hostId]);
  const newComposeHref = `/nodes/${encodeURIComponent(hostNodeID)}/new-compose`;
  const newContainerHref = `/nodes/${encodeURIComponent(hostNodeID)}/new-container`;
  const sortedStacks = useMemo(
    () => [...stacks].sort((a, b) => a.name.localeCompare(b.name)),
    [stacks]
  );

  return (
    <Card className="mb-4">
      <div className="mb-4 rounded-xl border border-[var(--line)] bg-[linear-gradient(135deg,var(--panel)_0%%,color-mix(in_oklab,var(--accent)_9%%,var(--panel))_100%%)] p-4">
        <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-wider text-[var(--muted)]">Compose Stacks</p>
            <p className="mt-1 text-sm text-[var(--text)]">
              Use a dedicated compose editor to deploy new stacks, then manage lifecycle here.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Link
              href={newComposeHref}
              className="inline-flex items-center justify-center rounded-lg bg-[var(--control-bg-active)] px-3 py-1.5 text-xs font-medium text-[var(--control-fg-active)] transition-opacity hover:opacity-90"
            >
              New Compose
            </Link>
            <Link
              href={newContainerHref}
              className="inline-flex items-center justify-center rounded-lg border border-[var(--control-border)] px-3 py-1.5 text-xs font-medium text-[var(--control-fg)] transition-colors hover:bg-[var(--control-bg-hover)]"
            >
              New Container
            </Link>
          </div>
        </div>
      </div>

      <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Compose Inventory ({sortedStacks.length})</h2>
      {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}

      {loading ? (
        <p className="text-sm text-[var(--muted)]">Loading stacks...</p>
      ) : sortedStacks.length > 0 ? (
        <div className="space-y-3">
          {sortedStacks.map((stack) => {
            const targetStackID = stackAssetId(hostId, stack.name);
            const isRunning = stack.status.trim().toLowerCase().startsWith("running");
            return (
              <div key={stack.name} className="rounded-lg border border-[var(--line)] p-3">
                <div className="mb-2 flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Link
                      href={`/nodes/${encodeURIComponent(targetStackID)}`}
                      className="text-sm font-medium text-[var(--accent)] hover:underline"
                    >
                      {stack.name}
                    </Link>
                    <Badge status={isRunning ? "ok" : "bad"} size="sm" />
                    <span className="text-[10px] uppercase tracking-wide text-[var(--muted)]">{stack.status}</span>
                    <span className="text-[10px] text-[var(--muted)]">{stack.container_ids?.length ?? 0} containers</span>
                  </div>
                  <div className="flex gap-1">
                    <Button
                      size="sm"
                      disabled={actionInFlight === targetStackID}
                      onClick={() => void runAction(targetStackID, "stack.up")}
                    >
                      Up
                    </Button>
                    <Button
                      size="sm"
                      disabled={actionInFlight === targetStackID}
                      onClick={() => void runAction(targetStackID, "stack.restart")}
                    >
                      Restart
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      disabled={actionInFlight === targetStackID}
                      onClick={() => void runAction(targetStackID, "stack.down")}
                    >
                      Down
                    </Button>
                  </div>
                </div>
                {stack.config_file ? (
                  <p className="truncate font-mono text-[10px] text-[var(--muted)]">{stack.config_file}</p>
                ) : null}
              </div>
            );
          })}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center gap-2 py-12">
          <p className="text-sm font-medium text-[var(--text)]">No stacks</p>
          <p className="text-xs text-[var(--muted)]">No Compose stacks found on this Docker host.</p>
          <Link
            href={newComposeHref}
            className="mt-2 inline-flex items-center justify-center rounded-lg bg-[var(--control-bg-active)] px-3 py-1.5 text-xs font-medium text-[var(--control-fg-active)] transition-opacity hover:opacity-90"
          >
            Deploy First Compose
          </Link>
        </div>
      )}
    </Card>
  );
}

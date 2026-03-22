"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Card } from "../../../../components/ui/Card";
import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import type { DockerContainer, DockerStack } from "../../../../../lib/docker";
import { executeStackAction, fetchDockerContainers, fetchDockerStacks } from "../../../../../lib/docker";

type Props = {
  hostId: string;
  stackName: string;
  stackAssetId: string;
};

export function DockerStackOverviewTab({ hostId, stackName, stackAssetId }: Props) {
  const [stack, setStack] = useState<DockerStack | null>(null);
  const [containers, setContainers] = useState<DockerContainer[]>([]);
  const [loading, setLoading] = useState(true);
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
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
      setError(err instanceof Error ? err.message : "failed to load stack details");
    } finally {
      setLoading(false);
    }
  }, [hostId, stackName]);

  useEffect(() => {
    void load();
    const timer = setInterval(() => void load(), 10_000);
    return () => clearInterval(timer);
  }, [load]);

  const isRunning = (stack?.status ?? "").trim().toLowerCase().startsWith("running");
  const memberContainers = useMemo(() => {
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

  async function runAction(action: string) {
    setActionInFlight(action);
    setError(null);
    try {
      await executeStackAction(stackAssetId, action);
      setTimeout(() => void load(), 1200);
    } catch (err) {
      setError(err instanceof Error ? err.message : "stack action failed");
    } finally {
      setActionInFlight(null);
    }
  }

  return (
    <Card className="mb-4">
      <div className="mb-4 rounded-xl border border-[var(--line)] bg-[linear-gradient(135deg,var(--panel)_0%%,color-mix(in_oklab,var(--accent)_10%%,var(--panel))_100%%)] p-4">
        <p className="text-xs font-semibold uppercase tracking-wider text-[var(--muted)]">Compose Stack</p>
        <div className="mt-2 flex items-center gap-2">
          <p className="text-lg font-semibold text-[var(--text)]">{stackName}</p>
          {stack ? <Badge status={isRunning ? "ok" : "bad"} size="sm" /> : null}
          {stack ? (
            <span className="text-[10px] uppercase tracking-wide text-[var(--muted)]">{stack.status}</span>
          ) : null}
        </div>
        <p className="mt-2 text-xs text-[var(--muted)]">
          Quick lifecycle controls and stack metadata.
        </p>
      </div>

      {loading ? (
        <p className="text-sm text-[var(--muted)]">Loading stack details...</p>
      ) : stack ? (
        <div className="space-y-4">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
            <div className="rounded-lg border border-[var(--line)] p-3">
              <p className="text-[10px] uppercase tracking-wider text-[var(--muted)]">Status</p>
              <p className="mt-1 text-sm font-medium text-[var(--text)]">{stack.status}</p>
            </div>
            <div className="rounded-lg border border-[var(--line)] p-3">
              <p className="text-[10px] uppercase tracking-wider text-[var(--muted)]">Containers</p>
              <p className="mt-1 text-sm font-medium text-[var(--text)]">{memberContainers.length}</p>
            </div>
            <div className="rounded-lg border border-[var(--line)] p-3">
              <p className="text-[10px] uppercase tracking-wider text-[var(--muted)]">Host</p>
              <p className="mt-1 text-sm font-medium text-[var(--text)] font-mono">{hostId}</p>
            </div>
          </div>

          <div className="rounded-lg border border-[var(--line)] p-3">
            <p className="text-[10px] uppercase tracking-wider text-[var(--muted)]">Compose File</p>
            <p className="mt-1 text-xs text-[var(--text)] font-mono break-all">{stack.config_file || "--"}</p>
          </div>

          <div className="flex items-center gap-2">
            <Button
              size="sm"
              disabled={actionInFlight === "stack.up"}
              onClick={() => void runAction("stack.up")}
            >
              Up
            </Button>
            <Button
              size="sm"
              disabled={actionInFlight === "stack.restart"}
              onClick={() => void runAction("stack.restart")}
            >
              Restart
            </Button>
            <Button
              size="sm"
              variant="danger"
              disabled={actionInFlight === "stack.down"}
              onClick={() => void runAction("stack.down")}
            >
              Down
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center gap-2 py-12">
          <p className="text-sm font-medium text-[var(--text)]">Stack not found</p>
          <p className="text-xs text-[var(--muted)]">This stack may have been removed or renamed.</p>
        </div>
      )}

      {error ? <p className="mt-3 text-xs text-[var(--bad)]">{error}</p> : null}
    </Card>
  );
}


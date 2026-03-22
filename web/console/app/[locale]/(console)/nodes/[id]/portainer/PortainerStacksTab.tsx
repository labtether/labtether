"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Badge } from "../../../../../components/ui/Badge";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { portainerAction, portainerFetch } from "./usePortainerData";

type PortainerStack = {
  Id: number;
  Name: string;
  Status: number;
  Env?: Array<{ name: string; value: string }>;
  // Status: 1 = active, 2 = inactive
};

function stackBadgeStatus(status: number): "ok" | "pending" | "bad" {
  if (status === 1) return "ok";
  return "bad";
}

function stackStatusLabel(status: number): string {
  if (status === 1) return "active";
  if (status === 2) return "inactive";
  return String(status);
}

type Props = {
  assetId: string;
};

export function PortainerStacksTab({ assetId }: Props) {
  const [stacks, setStacks] = useState<PortainerStack[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const seqRef = useRef(0);
  const latestRef = useRef(0);

  const load = useCallback(async () => {
    const id = ++seqRef.current;
    latestRef.current = id;
    setLoading(true);
    setError(null);
    try {
      const data = await portainerFetch<PortainerStack[]>(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/stacks`,
      );
      if (latestRef.current !== id) return;
      setStacks(data ?? []);
    } catch (err) {
      if (latestRef.current !== id) return;
      setError(err instanceof Error ? err.message : "failed to load stacks");
    } finally {
      if (latestRef.current === id) setLoading(false);
    }
  }, [assetId]);

  useEffect(() => {
    void load();
  }, [load]);

  const doAction = useCallback(async (stackId: number, action: "start" | "stop" | "redeploy" | "remove") => {
    setActionError(null);
    setActionInFlight(`${stackId}-${action}`);
    try {
      const method = action === "remove" ? "DELETE" : "POST";
      await portainerAction(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/stacks/${encodeURIComponent(stackId)}/${action}`,
        method,
      );
      await load();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : `failed to ${action} stack`);
    } finally {
      setActionInFlight(null);
    }
  }, [assetId, load]);

  if (loading && stacks.length === 0) {
    return <Card><p className="text-sm text-[var(--muted)]">Loading stacks…</p></Card>;
  }

  if (error && stacks.length === 0) {
    return <Card><p className="text-sm text-[var(--bad)]">{error}</p></Card>;
  }

  return (
    <Card>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-medium text-[var(--text)]">Stacks</h2>
        <Button size="sm" variant="ghost" onClick={() => { void load(); }} disabled={loading}>
          {loading ? "Refreshing…" : "Refresh"}
        </Button>
      </div>
      {actionError && (
        <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p>
      )}
      {stacks.length === 0 ? (
        <p className="text-sm text-[var(--muted)]">No stacks found.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-2 text-left font-medium text-[var(--muted)]">Name</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Status</th>
                <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--line)]">
              {stacks.map((s) => (
                <tr key={s.Id}>
                  <td className="py-2 font-medium text-[var(--text)]">{s.Name}</td>
                  <td className="py-2">
                    <div className="flex items-center gap-2">
                      <Badge status={stackBadgeStatus(s.Status)} size="sm" />
                      <span className="text-[var(--muted)]">{stackStatusLabel(s.Status)}</span>
                    </div>
                  </td>
                  <td className="py-2">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!!actionInFlight || s.Status === 1}
                        loading={actionInFlight === `${s.Id}-start`}
                        onClick={() => { void doAction(s.Id, "start"); }}
                      >
                        Start
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!!actionInFlight || s.Status !== 1}
                        loading={actionInFlight === `${s.Id}-stop`}
                        onClick={() => { void doAction(s.Id, "stop"); }}
                      >
                        Stop
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!!actionInFlight}
                        loading={actionInFlight === `${s.Id}-redeploy`}
                        onClick={() => { void doAction(s.Id, "redeploy"); }}
                      >
                        Redeploy
                      </Button>
                      <Button
                        size="sm"
                        variant="danger"
                        disabled={!!actionInFlight}
                        loading={actionInFlight === `${s.Id}-remove`}
                        onClick={() => { void doAction(s.Id, "remove"); }}
                      >
                        Remove
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}

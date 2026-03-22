"use client";

import { useCallback, useRef, useState } from "react";
import { Badge } from "../../../../../components/ui/Badge";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type TrueNASService = {
  name: string;
  running: boolean;
  enabled: boolean;
  uptime?: string;
};

const PRIORITY_SERVICES = new Set(["smb", "nfs", "ssh", "iscsi"]);

function runningBadge(running: boolean): "ok" | "bad" {
  return running ? "ok" : "bad";
}

type Props = {
  assetId: string;
};

export function TrueNASServicesTab({ assetId }: Props) {
  const { data: services, loading, error, refresh } = useTrueNASList<TrueNASService>(
    assetId,
    "services",
  );

  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const actionSeq = useRef(0);

  const doAction = useCallback(
    async (serviceName: string, action: "start" | "stop" | "restart") => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`${action}-${serviceName}`);
      try {
        await truenasAction(
          `/api/truenas/assets/${encodeURIComponent(assetId)}/services/${encodeURIComponent(serviceName)}/${action}`,
          "POST",
        );
        if (actionSeq.current === seq) refresh();
      } catch (err) {
        if (actionSeq.current === seq) {
          setActionError(err instanceof Error ? err.message : `${action} failed`);
        }
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, refresh],
  );

  const sorted = [...(Array.isArray(services) ? services : [])].sort((a, b) => {
    const aPriority = PRIORITY_SERVICES.has(a.name.toLowerCase()) ? 0 : 1;
    const bPriority = PRIORITY_SERVICES.has(b.name.toLowerCase()) ? 0 : 1;
    if (aPriority !== bPriority) return aPriority - bPriority;
    return a.name.localeCompare(b.name);
  });

  if (loading && services.length === 0) {
    return <Card><p className="text-sm text-[var(--muted)]">Loading services…</p></Card>;
  }

  if (error && services.length === 0) {
    return <Card><p className="text-sm text-[var(--bad)]">{error}</p></Card>;
  }

  return (
    <Card>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-medium text-[var(--text)]">Services</h2>
        <Button size="sm" variant="ghost" onClick={refresh} disabled={loading}>
          {loading ? "Refreshing…" : "Refresh"}
        </Button>
      </div>
      {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}
      {sorted.length === 0 ? (
        <p className="text-sm text-[var(--muted)]">No services found.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-2 text-left font-medium text-[var(--muted)]">Service</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Running</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Enabled</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Uptime</th>
                <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--line)]">
              {sorted.map((svc) => (
                <tr key={svc.name}>
                  <td className="py-2 font-medium text-[var(--text)]">{svc.name}</td>
                  <td className="py-2">
                    <div className="flex items-center gap-2">
                      <Badge status={runningBadge(svc.running)} size="sm" dot />
                      <span className="text-[var(--muted)]">{svc.running ? "Running" : "Stopped"}</span>
                    </div>
                  </td>
                  <td className="py-2">
                    <div className="flex items-center gap-2">
                      <Badge status={runningBadge(svc.enabled)} size="sm" dot />
                      <span className="text-[var(--muted)]">{svc.enabled ? "Yes" : "No"}</span>
                    </div>
                  </td>
                  <td className="py-2 text-[var(--muted)]">{svc.uptime ?? "--"}</td>
                  <td className="py-2 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!!actionInFlight || svc.running}
                        loading={actionInFlight === `start-${svc.name}`}
                        onClick={() => { void doAction(svc.name, "start"); }}
                      >
                        Start
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!!actionInFlight || !svc.running}
                        loading={actionInFlight === `stop-${svc.name}`}
                        onClick={() => { void doAction(svc.name, "stop"); }}
                      >
                        Stop
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!!actionInFlight}
                        loading={actionInFlight === `restart-${svc.name}`}
                        onClick={() => { void doAction(svc.name, "restart"); }}
                      >
                        Restart
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

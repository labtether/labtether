"use client";

import { useCallback, useRef, useState } from "react";
import { Badge } from "../../../../../components/ui/Badge";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type TrueNASVM = {
  id: string | number;
  name: string;
  status?: string;
  vcpus?: number;
  memory?: number;
  autostart?: boolean;
};

function vmStatusBadge(status?: string): "ok" | "pending" | "bad" {
  if (!status) return "pending";
  const s = status.toUpperCase();
  if (s === "RUNNING") return "ok";
  if (s === "SUSPENDED") return "pending";
  return "bad";
}

type Props = {
  assetId: string;
};

export function TrueNASVMsTab({ assetId }: Props) {
  const { data: vms, loading, error, refresh } = useTrueNASList<TrueNASVM>(assetId, "vms");
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const actionSeq = useRef(0);

  const doAction = useCallback(
    async (vmId: string | number, action: "start" | "stop" | "restart") => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`${action}-${vmId}`);
      try {
        await truenasAction(
          `/api/truenas/assets/${encodeURIComponent(assetId)}/vms/${encodeURIComponent(String(vmId))}/${action}`,
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

  if (loading && vms.length === 0) {
    return <Card><p className="text-sm text-[var(--muted)]">Loading VMs…</p></Card>;
  }

  // Surface CORE/SCALE mismatch warning
  if (error) {
    const isScaleOnly =
      error.toLowerCase().includes("scale") || error.toLowerCase().includes("core");
    if (isScaleOnly) {
      return (
        <Card>
          <p className="text-sm text-[var(--muted)]">
            VMs are only available on TrueNAS SCALE. This asset may be running TrueNAS CORE.
          </p>
        </Card>
      );
    }
    if (vms.length === 0) {
      return <Card><p className="text-sm text-[var(--bad)]">{error}</p></Card>;
    }
  }

  return (
    <Card>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-medium text-[var(--text)]">Virtual Machines</h2>
        <Button size="sm" variant="ghost" onClick={refresh} disabled={loading}>
          {loading ? "Refreshing…" : "Refresh"}
        </Button>
      </div>
      {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}
      {vms.length === 0 ? (
        <p className="text-sm text-[var(--muted)]">No virtual machines found.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-2 text-left font-medium text-[var(--muted)]">Name</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Status</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">vCPUs</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Memory</th>
                <th className="py-2 text-left font-medium text-[var(--muted)]">Autostart</th>
                <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--line)]">
              {vms.map((vm) => {
                const isRunning =
                  (vm.status ?? "").toUpperCase() === "RUNNING";
                return (
                  <tr key={String(vm.id)}>
                    <td className="py-2 font-medium text-[var(--text)]">{vm.name}</td>
                    <td className="py-2">
                      <div className="flex items-center gap-2">
                        <Badge status={vmStatusBadge(vm.status)} size="sm" />
                        <span className="text-[var(--muted)]">{vm.status ?? "--"}</span>
                      </div>
                    </td>
                    <td className="py-2 text-[var(--muted)]">{vm.vcpus ?? "--"}</td>
                    <td className="py-2 text-[var(--muted)]">
                      {vm.memory != null ? `${vm.memory} MiB` : "--"}
                    </td>
                    <td className="py-2 text-[var(--muted)]">
                      {vm.autostart != null ? (vm.autostart ? "Yes" : "No") : "--"}
                    </td>
                    <td className="py-2 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!actionInFlight || isRunning}
                          loading={actionInFlight === `start-${vm.id}`}
                          onClick={() => { void doAction(vm.id, "start"); }}
                        >
                          Start
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!actionInFlight || !isRunning}
                          loading={actionInFlight === `stop-${vm.id}`}
                          onClick={() => { void doAction(vm.id, "stop"); }}
                        >
                          Stop
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!actionInFlight}
                          loading={actionInFlight === `restart-${vm.id}`}
                          onClick={() => { void doAction(vm.id, "restart"); }}
                        >
                          Restart
                        </Button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}

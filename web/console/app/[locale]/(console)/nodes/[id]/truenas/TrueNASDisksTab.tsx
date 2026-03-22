"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Badge } from "../../../../../components/ui/Badge";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { TrueNASSmartHealthCard } from "../TrueNASSmartHealthCard";
import {
  diskStatusBadge,
  formatBytes,
  normalizeTrueNASSmartResponse,
} from "../truenasTabModel";
import type { TrueNASSmartResponse } from "../truenasTabModel";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type TrueNASDisk = {
  name: string;
  model?: string;
  serial?: string;
  type?: string;
  size?: number;
  temperature_celsius?: number;
  smart_status?: string;
  pool?: string;
};

function tempClass(temp: number): string {
  if (temp > 50) return "text-[var(--bad)]";
  if (temp >= 40) return "text-[var(--warn)]";
  return "text-[var(--ok)]";
}

type Props = {
  assetId: string;
};

export function TrueNASDisksTab({ assetId }: Props) {
  const { data: disks, loading: disksLoading, error: disksError, refresh: refreshDisks } =
    useTrueNASList<TrueNASDisk>(assetId, "disks");

  // SMART health card state — mirrors TrueNASTab.tsx fetch logic
  const [smart, setSmart] = useState<TrueNASSmartResponse | null>(null);
  const [smartLoading, setSmartLoading] = useState(false);
  const [smartError, setSmartError] = useState<string | null>(null);
  const smartInFlightRef = useRef(false);

  const fetchSmart = useCallback(async () => {
    if (smartInFlightRef.current) return;
    smartInFlightRef.current = true;
    setSmartLoading(true);
    setSmartError(null);
    try {
      const res = await fetch(
        `/api/truenas/assets/${encodeURIComponent(assetId)}/smart`,
        { cache: "no-store" },
      );
      const payload = normalizeTrueNASSmartResponse(await res.json().catch(() => null));
      if (!res.ok) throw new Error(payload.error ?? `failed to load disk health (${res.status})`);
      setSmart(payload);
    } catch (err) {
      setSmartError(err instanceof Error ? err.message : "failed to load disk health");
      setSmart(null);
    } finally {
      smartInFlightRef.current = false;
      setSmartLoading(false);
    }
  }, [assetId]);

  useEffect(() => {
    void fetchSmart();
    const interval = setInterval(() => { void fetchSmart(); }, 60_000);
    return () => clearInterval(interval);
  }, [fetchSmart]);

  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const actionSeq = useRef(0);

  const doSmartTest = useCallback(
    async (diskName: string, testType: "short" | "long") => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`smart-${diskName}-${testType}`);
      try {
        await truenasAction(
          `/api/truenas/assets/${encodeURIComponent(assetId)}/disks/${encodeURIComponent(diskName)}/smart-test`,
          "POST",
          { type: testType },
        );
        if (actionSeq.current === seq) refreshDisks();
      } catch (err) {
        if (actionSeq.current === seq) {
          setActionError(err instanceof Error ? err.message : "SMART test failed");
        }
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, refreshDisks],
  );

  return (
    <div className="space-y-4">
      <TrueNASSmartHealthCard
        loading={smartLoading}
        error={smartError}
        summary={smart?.summary}
        warnings={smart?.warnings ?? []}
        disks={smart?.disks ?? []}
        onRefresh={() => { void fetchSmart(); }}
      />

      <Card>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-[var(--text)]">All Disks</h2>
          <Button size="sm" variant="ghost" onClick={refreshDisks} disabled={disksLoading}>
            {disksLoading ? "Refreshing…" : "Refresh"}
          </Button>
        </div>
        {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}
        {disksError && disks.length === 0 ? (
          <p className="text-sm text-[var(--bad)]">{disksError}</p>
        ) : disksLoading && disks.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">Loading disks…</p>
        ) : disks.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No disks found.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Name</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Model</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Serial</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Type</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Size</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Temp</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">SMART</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Pool</th>
                  <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--line)]">
                {disks.map((disk) => (
                  <tr key={disk.name}>
                    <td className="py-2 font-medium text-[var(--text)]">{disk.name}</td>
                    <td className="py-2 text-[var(--muted)]">{disk.model ?? "--"}</td>
                    <td className="py-2 text-[var(--muted)]">{disk.serial ?? "--"}</td>
                    <td className="py-2 text-[var(--muted)]">{disk.type ?? "--"}</td>
                    <td className="py-2 text-[var(--muted)]">
                      {disk.size != null ? formatBytes(disk.size) : "--"}
                    </td>
                    <td className={`py-2 font-medium ${disk.temperature_celsius != null ? tempClass(disk.temperature_celsius) : "text-[var(--muted)]"}`}>
                      {disk.temperature_celsius != null
                        ? `${disk.temperature_celsius.toFixed(1)}°C`
                        : "--"}
                    </td>
                    <td className="py-2">
                      {disk.smart_status ? (
                        <div className="flex items-center gap-2">
                          <Badge status={diskStatusBadge(disk.smart_status)} size="sm" />
                          <span className="text-[var(--muted)]">{disk.smart_status}</span>
                        </div>
                      ) : (
                        <span className="text-[var(--muted)]">--</span>
                      )}
                    </td>
                    <td className="py-2 text-[var(--muted)]">{disk.pool ?? "--"}</td>
                    <td className="py-2 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!actionInFlight}
                          loading={actionInFlight === `smart-${disk.name}-short`}
                          onClick={() => { void doSmartTest(disk.name, "short"); }}
                        >
                          Short
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!actionInFlight}
                          loading={actionInFlight === `smart-${disk.name}-long`}
                          onClick={() => { void doSmartTest(disk.name, "long"); }}
                        >
                          Long
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
    </div>
  );
}

"use client";

import { useState } from "react";
import { Card } from "../../../../../components/ui/Card";
import { useProxmoxFetch } from "./useProxmoxData";

type MetricPoint = {
  time?: number;
  cpu?: number;
  mem?: number;
  maxmem?: number;
  disk?: number;
  maxdisk?: number;
  netin?: number;
  netout?: number;
  diskread?: number;
  diskwrite?: number;
};

type MetricsResponse = {
  data?: MetricPoint[];
  timeframe?: string;
};

const WINDOWS = ["hour", "day", "week", "month"] as const;
type Window = (typeof WINDOWS)[number];

const WINDOW_LABELS: Record<Window, string> = {
  hour: "1h",
  day: "24h",
  week: "7d",
  month: "30d",
};

type Props = {
  assetId: string;
  proxmoxCollectorID: string;
};

function fmtPct(v: number | undefined): string {
  if (v == null) return "-";
  return `${(v * 100).toFixed(1)}%`;
}

function fmtBytes(v: number | undefined): string {
  if (v == null) return "-";
  if (v >= 1_073_741_824) return `${(v / 1_073_741_824).toFixed(1)} GB`;
  if (v >= 1_048_576) return `${(v / 1_048_576).toFixed(1)} MB`;
  if (v >= 1024) return `${(v / 1024).toFixed(1)} KB`;
  return `${v} B`;
}

export function ProxmoxMetricsTab({ assetId, proxmoxCollectorID }: Props) {
  const [window_, setWindow] = useState<Window>("hour");

  const collectorSuffix = proxmoxCollectorID
    ? `&collector_id=${encodeURIComponent(proxmoxCollectorID)}`
    : "";

  const path = `/api/proxmox/assets/${encodeURIComponent(assetId)}/metrics?timeframe=${window_}${collectorSuffix}`;
  const { data, loading, error, refresh } = useProxmoxFetch<MetricsResponse>(path);

  const points: MetricPoint[] = data?.data ?? (Array.isArray(data) ? (data as MetricPoint[]) : []);

  // Show last N points in table (most recent last)
  const displayPoints = points.slice(-50);

  return (
    <Card>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <h2 className="text-sm font-medium text-[var(--text)]">Metrics</h2>
        <div className="ml-auto flex items-center gap-2">
          <div className="flex rounded border border-[var(--line)] overflow-hidden">
            {WINDOWS.map((w) => (
              <button
                key={w}
                onClick={() => { setWindow(w); }}
                className={`px-2 py-1 text-xs transition-colors ${
                  window_ === w
                    ? "bg-[var(--accent)] text-white"
                    : "bg-[var(--surface)] text-[var(--muted)] hover:text-[var(--text)]"
                }`}
              >
                {WINDOW_LABELS[w]}
              </button>
            ))}
          </div>
          <button
            className="text-xs text-[var(--accent)] hover:underline"
            onClick={refresh}
            disabled={loading}
          >
            {loading ? "Loading..." : "Refresh"}
          </button>
        </div>
      </div>
      {error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : loading && displayPoints.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">Loading metrics...</p>
      ) : displayPoints.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Time</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">CPU</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Mem Used</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Mem Total</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Disk Used</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Net In</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Net Out</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Disk Read</th>
                <th className="px-2 py-1 text-left font-medium text-[var(--muted)]">Disk Write</th>
              </tr>
            </thead>
            <tbody>
              {displayPoints.map((pt, idx) => (
                <tr key={pt.time ?? idx} className="border-b border-[var(--line)] border-opacity-30">
                  <td className="px-2 py-1 text-[var(--muted)]">
                    {pt.time ? new Date(pt.time * 1000).toLocaleTimeString() : "-"}
                  </td>
                  <td className="px-2 py-1 text-[var(--text)]">{fmtPct(pt.cpu)}</td>
                  <td className="px-2 py-1 text-[var(--text)]">{fmtBytes(pt.mem)}</td>
                  <td className="px-2 py-1 text-[var(--muted)]">{fmtBytes(pt.maxmem)}</td>
                  <td className="px-2 py-1 text-[var(--muted)]">{fmtBytes(pt.disk)}</td>
                  <td className="px-2 py-1 text-[var(--muted)]">{fmtBytes(pt.netin)}</td>
                  <td className="px-2 py-1 text-[var(--muted)]">{fmtBytes(pt.netout)}</td>
                  <td className="px-2 py-1 text-[var(--muted)]">{fmtBytes(pt.diskread)}</td>
                  <td className="px-2 py-1 text-[var(--muted)]">{fmtBytes(pt.diskwrite)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="text-xs text-[var(--muted)]">No metrics data available for this time range.</p>
      )}
    </Card>
  );
}

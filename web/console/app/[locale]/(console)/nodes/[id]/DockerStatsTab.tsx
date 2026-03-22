"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Card } from "../../../../components/ui/Card";
import { formatBytes } from "../../../../console/formatters";
import { useDocumentVisibility } from "../../../../hooks/useDocumentVisibility";
import { fetchDockerContainerStats } from "../../../../../lib/docker";

type ContainerStats = {
  cpu_percent: number;
  memory_bytes: number;
  memory_limit: number;
  memory_percent: number;
  net_rx_bytes: number;
  net_tx_bytes: number;
  block_read_bytes: number;
  block_write_bytes: number;
  pids: number;
};

type Props = { containerId: string };

export function DockerStatsTab({ containerId }: Props) {
  const [stats, setStats] = useState<ContainerStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [history, setHistory] = useState<{ cpu: number; mem: number; ts: number }[]>([]);
  const [error, setError] = useState<string | null>(null);
  const isDocumentVisible = useDocumentVisibility();
  const loadInFlightRef = useRef(false);

  const load = useCallback(async () => {
    if (loadInFlightRef.current) {
      return;
    }
    loadInFlightRef.current = true;
    try {
      const raw = await fetchDockerContainerStats(containerId) as Partial<ContainerStats>;
      if (typeof raw.cpu_percent !== "number") {
        setStats(null);
        setHistory([]);
        setError(null);
        return;
      }
      const data: ContainerStats = {
        cpu_percent: raw.cpu_percent ?? 0,
        memory_bytes: raw.memory_bytes ?? 0,
        memory_limit: raw.memory_limit ?? 0,
        memory_percent: raw.memory_percent ?? 0,
        net_rx_bytes: raw.net_rx_bytes ?? 0,
        net_tx_bytes: raw.net_tx_bytes ?? 0,
        block_read_bytes: raw.block_read_bytes ?? 0,
        block_write_bytes: raw.block_write_bytes ?? 0,
        pids: raw.pids ?? 0,
      };
      setStats(data);
      setHistory((prev) => {
        const next = [...prev, { cpu: data.cpu_percent, mem: data.memory_percent, ts: Date.now() }];
        return next.slice(-60); // Keep last 60 samples (5 min at 5s intervals)
      });
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load stats");
    } finally {
      loadInFlightRef.current = false;
      setLoading(false);
    }
  }, [containerId]);

  useEffect(() => {
    if (!isDocumentVisible) {
      return;
    }
    void load();
    const interval = setInterval(() => void load(), 5_000);
    return () => clearInterval(interval);
  }, [isDocumentVisible, load]);

  const maxCpu = useMemo(
    () => history.reduce((max, point) => Math.max(max, point.cpu), 1),
    [history],
  );

  const cpuBarHeights = useMemo(
    () => history.map((point) => Math.max(2, (point.cpu / maxCpu) * 60)),
    [history, maxCpu],
  );

  return (
    <Card className="mb-4">
      <h2 className="text-sm font-medium text-[var(--text)] mb-3">Container Stats</h2>
      {loading && !stats ? (
        <p className="text-sm text-[var(--muted)]">Loading stats...</p>
      ) : error && !stats ? (
        <div className="flex flex-col items-center justify-center py-12 gap-2">
          <p className="text-sm font-medium text-[var(--bad)]">Failed to load stats</p>
          <p className="text-xs text-[var(--muted)]">{error}</p>
        </div>
      ) : stats ? (
        <div className="space-y-4">
          {/* Gauge row */}
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <StatCard label="CPU" value={`${stats.cpu_percent.toFixed(1)}%`} color={gaugeColor(stats.cpu_percent)} />
            <StatCard label="Memory" value={`${stats.memory_percent.toFixed(1)}%`} sub={`${formatBytes(stats.memory_bytes)} / ${formatBytes(stats.memory_limit)}`} color={gaugeColor(stats.memory_percent)} />
            <StatCard label="Net I/O" value={`${formatBytes(stats.net_rx_bytes)} / ${formatBytes(stats.net_tx_bytes)}`} sub="RX / TX" />
            <StatCard label="PIDs" value={String(stats.pids)} />
          </div>

          {/* Mini sparkline chart */}
          {history.length > 1 ? (
            <div className="space-y-2">
              <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">CPU History</p>
              <div className="flex items-end gap-px h-16">
                {history.map((pt, i) => {
                  return (
                    <div
                      key={i}
                      className="flex-1 bg-[var(--accent)] rounded-sm opacity-60"
                      style={{ height: `${cpuBarHeights[i] ?? 2}px` }}
                      title={`CPU: ${pt.cpu.toFixed(1)}%`}
                    />
                  );
                })}
              </div>
            </div>
          ) : null}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center py-12 gap-2">
          <p className="text-sm font-medium text-[var(--text)]">No stats</p>
          <p className="text-xs text-[var(--muted)]">Container may not be running.</p>
        </div>
      )}
    </Card>
  );
}

function StatCard({ label, value, sub, color }: { label: string; value: string; sub?: string; color?: string }) {
  return (
    <div className="border border-[var(--line)] rounded-lg p-3 space-y-1">
      <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">{label}</p>
      <p className={`text-lg font-semibold ${color ?? "text-[var(--text)]"}`}>{value}</p>
      {sub ? <p className="text-[10px] text-[var(--muted)]">{sub}</p> : null}
    </div>
  );
}

function gaugeColor(percent: number): string {
  if (percent > 90) return "text-[var(--bad)]";
  if (percent > 70) return "text-[var(--warn)]";
  return "text-[var(--ok)]";
}

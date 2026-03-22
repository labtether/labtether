"use client";

import { useMemo } from "react";
import { Link } from "../../../i18n/navigation";
import { Card } from "../ui/Card";
import { Badge } from "../ui/Badge";
import type { DockerHostSummary, DockerContainer } from "../../../lib/docker";
import { containerAssetID, hostAssetID } from "./containerUtils";

export type HostWithContainers = {
  host: DockerHostSummary;
  containers: DockerContainer[];
};

type Props = {
  hosts: HostWithContainers[];
};

function fmt(value: number | undefined, decimals = 1): string {
  if (value == null || !Number.isFinite(value)) return "--";
  return value.toFixed(decimals);
}

type TopNTableProps = {
  title: string;
  rows: { container: DockerContainer; host: DockerHostSummary }[];
  metricKey: "cpu_percent" | "memory_percent";
  metricLabel: string;
};

function TopNTable({ title, rows, metricKey, metricLabel }: TopNTableProps) {
  return (
    <Card variant="flush">
      <div className="px-4 py-3 border-b border-[var(--line)]">
        <h3 className="text-sm font-medium text-[var(--text)]">{title}</h3>
      </div>
      {rows.length === 0 ? (
        <p className="px-4 py-6 text-xs text-[var(--muted)] text-center">
          No metric data available yet.
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="px-4 py-2 text-left font-medium text-[var(--muted)]">Container</th>
                <th className="px-4 py-2 text-left font-medium text-[var(--muted)]">Host</th>
                <th className="px-4 py-2 text-left font-medium text-[var(--muted)] max-w-40 truncate">Image</th>
                <th className="px-4 py-2 text-right font-medium text-[var(--muted)]">{metricLabel}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--line)]">
              {rows.map(({ container, host }) => {
                const assetID = containerAssetID(host.agent_id, container.id);
                const metricValue = container[metricKey];
                return (
                  <tr key={`${host.agent_id}-${container.id}`} className="hover:bg-[var(--hover)]">
                    <td className="px-4 py-2 font-medium">
                      <Link
                        href={`/nodes/${encodeURIComponent(assetID)}`}
                        className="text-[var(--accent)] hover:underline"
                      >
                        {container.name}
                      </Link>
                    </td>
                    <td className="px-4 py-2 text-[var(--muted)]">
                      <Link
                        href={`/nodes/${encodeURIComponent(hostAssetID(host.agent_id))}`}
                        className="hover:underline hover:text-[var(--text)]"
                      >
                        {host.agent_id}
                      </Link>
                      {host.source === "portainer" && (
                        <span className="ml-1 rounded px-1 py-0.5 text-[10px] bg-[var(--accent-subtle)] text-[var(--accent-text)]">P</span>
                      )}
                    </td>
                    <td className="px-4 py-2 text-[var(--muted)] max-w-40 truncate">
                      {container.image}
                    </td>
                    <td className="px-4 py-2 text-right font-mono tabular-nums">
                      <Badge
                        status={metricValue != null && metricValue > 80 ? "bad" : metricValue != null && metricValue > 50 ? "pending" : "ok"}
                        size="sm"
                        dot
                      />
                      {" "}
                      {fmt(metricValue)}%
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

export function ContainerFleetOverview({ hosts }: Props) {
  const allContainers = useMemo(
    () => hosts.flatMap(({ host, containers }) =>
      containers.map((c) => ({ container: c, host }))
    ),
    [hosts]
  );

  const stats = useMemo(() => {
    let running = 0;
    let stopped = 0;
    let errored = 0;
    const cpuValues: number[] = [];
    const memValues: number[] = [];

    for (const { container } of allContainers) {
      const s = container.state.toLowerCase();
      if (s === "running") running++;
      else if (s === "exited" || s === "dead") errored++;
      else stopped++;

      if (container.cpu_percent != null) cpuValues.push(container.cpu_percent);
      if (container.memory_percent != null) memValues.push(container.memory_percent);
    }

    const avgCpu =
      cpuValues.length > 0
        ? cpuValues.reduce((a, b) => a + b, 0) / cpuValues.length
        : undefined;
    const avgMem =
      memValues.length > 0
        ? memValues.reduce((a, b) => a + b, 0) / memValues.length
        : undefined;

    return { running, stopped, errored, avgCpu, avgMem };
  }, [allContainers]);

  const topByCpu = useMemo(
    () =>
      [...allContainers]
        .filter(({ container }) => container.cpu_percent != null)
        .sort((a, b) => (b.container.cpu_percent ?? 0) - (a.container.cpu_percent ?? 0))
        .slice(0, 10),
    [allContainers]
  );

  const topByMem = useMemo(
    () =>
      [...allContainers]
        .filter(({ container }) => container.memory_percent != null)
        .sort((a, b) => (b.container.memory_percent ?? 0) - (a.container.memory_percent ?? 0))
        .slice(0, 10),
    [allContainers]
  );

  return (
    <div className="space-y-6">
      {/* Summary bar */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        <StatCard label="Containers" value={String(allContainers.length)} />
        <StatCard label="Docker Hosts" value={String(hosts.length)} />
        <StatCard
          label="Running"
          value={String(stats.running)}
          valueClass="text-[var(--ok)]"
        />
        <StatCard
          label="Avg CPU%"
          value={stats.avgCpu != null ? `${fmt(stats.avgCpu)}%` : "--"}
        />
        <StatCard
          label="Avg Mem%"
          value={stats.avgMem != null ? `${fmt(stats.avgMem)}%` : "--"}
        />
      </div>

      {/* State breakdown */}
      <div className="flex items-center gap-4 text-xs text-[var(--muted)]">
        <span>
          <span className="font-medium text-[var(--ok)]">{stats.running}</span> running
        </span>
        <span>
          <span className="font-medium text-[var(--text)]">{stats.stopped}</span> stopped
        </span>
        <span>
          <span className="font-medium text-[var(--bad)]">{stats.errored}</span> errored
        </span>
      </div>

      {/* Top-N tables */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <TopNTable
          title="Top 10 by CPU%"
          rows={topByCpu}
          metricKey="cpu_percent"
          metricLabel="CPU%"
        />
        <TopNTable
          title="Top 10 by Mem%"
          rows={topByMem}
          metricKey="memory_percent"
          metricLabel="Mem%"
        />
      </div>
    </div>
  );
}

function StatCard({
  label,
  value,
  valueClass = "text-[var(--text)]",
}: {
  label: string;
  value: string;
  valueClass?: string;
}) {
  return (
    <Card>
      <p className="text-[10px] font-mono uppercase tracking-wide text-[var(--muted)] mb-1">
        {label}
      </p>
      <p className={`text-2xl font-semibold font-[family-name:var(--font-heading)] tabular-nums ${valueClass}`}>
        {value}
      </p>
    </Card>
  );
}

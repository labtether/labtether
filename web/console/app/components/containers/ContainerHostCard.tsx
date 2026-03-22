"use client";

import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Link } from "../../../i18n/navigation";
import { Card } from "../ui/Card";
import { Badge } from "../ui/Badge";
import type { DockerHostSummary, DockerContainer } from "../../../lib/docker";
import { hostAssetID, containerAssetID, containerBadgeStatus } from "./containerUtils";

type Props = {
  host: DockerHostSummary;
  containers: DockerContainer[];
  defaultOpen?: boolean;
};

function fmt(value: number | undefined, decimals = 1): string {
  if (value == null || !Number.isFinite(value)) return "--";
  return value.toFixed(decimals);
}

export function ContainerHostCard({ host, containers, defaultOpen = false }: Props) {
  const [open, setOpen] = useState(defaultOpen);

  const runningCount = containers.filter((c) => c.state.toLowerCase() === "running").length;
  const stoppedCount = containers.length - runningCount;

  return (
    <Card variant="flush">
      {/* Collapsible header */}
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-[var(--hover)] transition-colors duration-[var(--dur-fast)]"
        aria-expanded={open}
      >
        <span className="text-[var(--muted)] flex-shrink-0">
          {open ? (
            <ChevronDown size={14} />
          ) : (
            <ChevronRight size={14} />
          )}
        </span>

        {/* Host name */}
        <span className="font-medium text-sm text-[var(--text)] flex-1 truncate">
          <Link
            href={`/nodes/${encodeURIComponent(hostAssetID(host.agent_id))}`}
            className="text-[var(--accent)] hover:underline"
            onClick={(e) => e.stopPropagation()}
          >
            {host.agent_id}
          </Link>
        </span>

        {/* Engine version */}
        {host.engine_version ? (
          <span className="text-[10px] font-mono text-[var(--muted)] hidden sm:block">
            Docker {host.engine_version}
          </span>
        ) : null}

        {/* Source badge */}
        <span className={`rounded-md px-1.5 py-0.5 text-[10px] font-medium ${
          host.source === "portainer"
            ? "bg-[var(--accent-subtle)] text-[var(--accent-text)]"
            : "bg-[var(--surface)] text-[var(--muted)]"
        }`}>
          {host.source === "portainer" ? "Portainer" : "Docker"}
        </span>

        {/* Container count with state breakdown */}
        <span className="text-xs text-[var(--muted)] flex-shrink-0">
          <span className="text-[var(--ok)] font-medium">{runningCount}</span>
          <span className="mx-0.5">/</span>
          <span className="font-medium">{containers.length}</span>
          <span className="ml-1">containers</span>
        </span>

        {stoppedCount > 0 ? (
          <Badge status="pending" size="sm" dot />
        ) : (
          <Badge status="ok" size="sm" dot />
        )}
      </button>

      {/* Collapsible body */}
      {open ? (
        <div className="border-t border-[var(--line)]">
          {containers.length === 0 ? (
            <p className="px-4 py-6 text-xs text-[var(--muted)] text-center">
              No containers on this host.
            </p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-[var(--line)]">
                    <th className="px-4 py-2 text-left font-medium text-[var(--muted)]">Name</th>
                    <th className="px-4 py-2 text-left font-medium text-[var(--muted)] max-w-48">Image</th>
                    <th className="px-4 py-2 text-left font-medium text-[var(--muted)]">State</th>
                    <th className="px-4 py-2 text-right font-medium text-[var(--muted)]">CPU%</th>
                    <th className="px-4 py-2 text-right font-medium text-[var(--muted)]">Mem%</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--line)]">
                  {containers.map((container) => {
                    const assetID = containerAssetID(host.agent_id, container.id);
                    return (
                      <tr key={container.id} className="hover:bg-[var(--hover)]">
                        <td className="px-4 py-2 font-medium">
                          <Link
                            href={`/nodes/${encodeURIComponent(assetID)}`}
                            className="text-[var(--accent)] hover:underline"
                          >
                            {container.name}
                          </Link>
                        </td>
                        <td className="px-4 py-2 text-[var(--muted)] max-w-48 truncate">
                          {container.image}
                        </td>
                        <td className="px-4 py-2">
                          <Badge status={containerBadgeStatus(container.state)} size="sm" />
                        </td>
                        <td className="px-4 py-2 text-right font-mono tabular-nums text-[var(--text)]">
                          {fmt(container.cpu_percent)}%
                        </td>
                        <td className="px-4 py-2 text-right font-mono tabular-nums text-[var(--text)]">
                          {fmt(container.memory_percent)}%
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      ) : null}
    </Card>
  );
}

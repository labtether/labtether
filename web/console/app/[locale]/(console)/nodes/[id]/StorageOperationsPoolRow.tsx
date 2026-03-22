"use client";

import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { formatBytes } from "../../../../console/formatters";
import type { StorageRow } from "./storageOperationsModel";
import {
  barColor,
  formatDays,
  formatGrowthBytes,
  formatPercent,
  isHealthyHealth,
  riskBadgeStatus,
} from "./storageOperationsUtils";

type StorageOperationsPoolRowProps = {
  row: StorageRow;
  isExpanded: boolean;
  onToggleExpanded: () => void;
};

export function StorageOperationsPoolRow({
  row,
  isExpanded,
  onToggleExpanded,
}: StorageOperationsPoolRowProps) {
  return (
    <tr className="border-b border-[var(--line)] border-opacity-30 align-top">
      <td className="py-2 pr-3">
        <div className="space-y-0.5">
          <p className="text-sm font-medium text-[var(--text)]">{row.poolName}</p>
          <p className="text-[10px] text-[var(--muted)] uppercase tracking-wider">{row.typeLabel}</p>
        </div>
      </td>
      <td className="py-2 pr-3">
        <span
          className={`inline-flex text-[10px] px-1.5 py-0.5 rounded-lg border ${
            isHealthyHealth(row.health)
              ? "border-[var(--ok)]/40 text-[var(--ok)]"
              : "border-[var(--bad)]/40 text-[var(--bad)]"
          }`}
        >
          {row.health}
        </span>
      </td>
      <td className="py-2 pr-3">
        <div className="min-w-[120px]">
          <div className="h-1.5 rounded-full bg-[var(--hover)] overflow-hidden mb-1">
            <div
              className={`h-full ${barColor(row.usedPercent)}`}
              style={{ width: `${row.usedPercent == null ? 0 : Math.max(Math.min(row.usedPercent, 100), 0)}%` }}
            />
          </div>
          <span className="text-xs text-[var(--text)]">{formatPercent(row.usedPercent)}</span>
        </div>
      </td>
      <td className="py-2 pr-3 text-[var(--muted)]">{row.freeBytes != null ? formatBytes(row.freeBytes) : "--"}</td>
      <td className="py-2 pr-3 text-[var(--muted)]">{formatGrowthBytes(row.growthBytes7d)}</td>
      <td className="py-2 pr-3 text-[var(--muted)]">{formatDays(row.daysTo80)}</td>
      <td className="py-2 pr-3 text-[var(--muted)]">{formatDays(row.daysToFull)}</td>
      <td className="py-2 pr-3 text-[var(--muted)]">{row.fragPercent != null ? `${row.fragPercent}%` : "--"}</td>
      <td className="py-2 pr-3 text-[var(--muted)]">{row.dedupRatio != null ? row.dedupRatio.toFixed(2) : "--"}</td>
      <td className="py-2 pr-3 text-[var(--muted)]">
        {row.snapshotCount > 0 ? `${row.snapshotCount} (${formatBytes(row.snapshotBytes)})` : "--"}
      </td>
      <td className="py-2 pr-3 text-[var(--muted)]">VM {row.vmCount} / CT {row.ctCount}</td>
      <td className="py-2 pr-3 text-[var(--muted)]">{row.stale ? row.staleLabel : "fresh"}</td>
      <td className="py-2">
        <div className="space-y-1">
          <Badge status={riskBadgeStatus(row.riskState)} size="sm" />
          <p className="text-[10px] text-[var(--muted)] max-w-[180px]">{row.reason}</p>
          <p className="text-[10px] text-[var(--muted)]">forecast confidence: {row.confidence}</p>
        </div>
      </td>
      <td className="py-2 pl-3">
        <Button size="sm" variant="ghost" onClick={onToggleExpanded}>
          {isExpanded ? "Hide" : "Details"}
        </Button>
      </td>
    </tr>
  );
}

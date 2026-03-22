"use client";

import { ChevronLeft, Pencil, Plus, RotateCw, Power, Trash2 } from "lucide-react";
import { Badge } from "./ui/Badge";
import { MiniBar } from "./ui/MiniBar";
import type { Asset, TelemetryOverviewAsset } from "../console/models";
import { friendlySourceLabel, sourceIcon } from "../console/taxonomy";

type DeviceIdentityBarProps = {
  asset: Asset;
  telemetry: TelemetryOverviewAsset | null;
  groupName: string;
  freshnessStatus: "ok" | "pending" | "bad";
  agentConnected: boolean;
  activePanel: string | null;
  onBack: () => void;
  onQuickAction?: (command: string) => void;
  onAddConnection?: () => void;
  onEdit?: () => void;
  onDelete?: () => void;
};

function metadataFallbackMetrics(
  meta?: Record<string, string>,
): Partial<TelemetryOverviewAsset["metrics"]> | undefined {
  if (!meta) return undefined;
  const result: Partial<TelemetryOverviewAsset["metrics"]> = {};

  const cpu = meta.cpu_used_percent ?? meta.cpu_percent;
  if (cpu && Number.isFinite(Number(cpu))) result.cpu_used_percent = Number(cpu);

  const memPct = meta.memory_used_percent ?? meta.memory_percent;
  if (memPct && Number.isFinite(Number(memPct))) {
    result.memory_used_percent = Number(memPct);
  } else {
    const total = Number(meta.physmem ?? meta.memory_total_bytes ?? "0");
    const avail = Number(meta.memory_available_bytes ?? "0");
    if (total > 0 && avail >= 0) {
      result.memory_used_percent = ((total - avail) / total) * 100;
    }
  }

  const diskPct = meta.disk_used_percent ?? meta.disk_percent;
  if (diskPct && Number.isFinite(Number(diskPct))) result.disk_used_percent = Number(diskPct);

  const temp = meta.temperature_celsius ?? meta.temp_celsius;
  if (temp && Number.isFinite(Number(temp))) result.temperature_celsius = Number(temp);

  return Object.keys(result).length > 0 ? result : undefined;
}

export function DeviceIdentityBar({
  asset,
  telemetry,
  groupName,
  freshnessStatus,
  agentConnected,
  activePanel,
  onBack,
  onQuickAction,
  onAddConnection,
  onEdit,
  onDelete,
}: DeviceIdentityBarProps) {
  const metrics = telemetry?.metrics ?? metadataFallbackMetrics(asset.metadata);

  const statusBadge =
    freshnessStatus === "ok"
      ? "online"
      : freshnessStatus === "pending"
        ? "stale"
        : "offline";

  const os =
    asset.metadata?.os_pretty_name || asset.metadata?.os_name || "";

  const platform = asset.platform || "";

  const sourceLabel = asset.source ? friendlySourceLabel(asset.source) : "";

  const SourceIcon = sourceIcon(asset.source);

  const quickActionButtonClass =
    "p-1.5 rounded-md hover:bg-[var(--surface)] text-[var(--muted)] hover:text-[var(--text)] transition-colors cursor-pointer";

  const metaItems: string[] = [];
  if (sourceLabel) metaItems.push(sourceLabel);
  if (platform) metaItems.push(platform);
  if (os) metaItems.push(os);
  if (groupName) metaItems.push(groupName);

  return (
    <div className="rounded-lg border border-[var(--line)] bg-[var(--panel-glass)] px-4 py-3 mb-4 space-y-1.5">
      {/* Row 1: Back button, device name, status badge */}
      <div className="flex items-center gap-2 min-w-0">
        {activePanel !== null && (
          <button
            onClick={onBack}
            className="p-1 rounded-md hover:bg-[var(--surface)] text-[var(--muted)] hover:text-[var(--text)] transition-colors shrink-0"
            style={{ transitionDuration: "var(--dur-fast)" }}
            aria-label="Back to dashboard"
          >
            <ChevronLeft size={16} />
          </button>
        )}
        <h2 className="text-lg font-semibold text-[var(--text)] truncate min-w-0 flex-1">
          {asset.name}
        </h2>
        <Badge status={statusBadge} size="sm" />
        {onAddConnection && (
          <button
            type="button"
            onClick={onAddConnection}
            className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] transition-colors shrink-0"
            style={{ transitionDuration: "var(--dur-fast)" }}
            aria-label="Add connection"
            title="Add connection"
          >
            <Plus size={14} />
          </button>
        )}
        {onEdit && (
          <button
            onClick={onEdit}
            className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] transition-colors shrink-0"
            style={{ transitionDuration: "var(--dur-fast)" }}
            aria-label="Edit device"
            title="Edit"
          >
            <Pencil size={14} />
          </button>
        )}
        {onDelete && (
          <button
            onClick={onDelete}
            className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--bad)] transition-colors shrink-0 ml-1"
            style={{ transitionDuration: "var(--dur-fast)" }}
            aria-label="Delete device"
            title="Delete device"
          >
            <Trash2 size={14} />
          </button>
        )}
      </div>

      {/* Row 2: Source, platform, OS, group, agent indicator */}
      <div className="flex items-center gap-1 flex-wrap">
        <SourceIcon size={11} className="text-[var(--muted)] shrink-0" />
        {metaItems.map((item, index) => (
          <span key={index} className="text-xs text-[var(--muted)] flex items-center gap-1">
            {index > 0 && <span aria-hidden="true">·</span>}
            {item}
          </span>
        ))}
        {agentConnected && (
          <span className="inline-flex items-center gap-1 text-xs text-[var(--muted)] ml-1">
            <span aria-hidden="true">·</span>
            <span
              className="inline-block h-1.5 w-1.5 rounded-full bg-[var(--ok)] shrink-0"
              style={{ boxShadow: "0 0 6px var(--ok-glow)" }}
            />
            <span style={{ color: "var(--ok)" }}>Agent Connected</span>
          </span>
        )}
      </div>

      {/* Row 3: MiniBar metrics (left) + quick action buttons (right) */}
      {((metrics?.cpu_used_percent != null ||
        metrics?.memory_used_percent != null ||
        metrics?.disk_used_percent != null) ||
        onQuickAction) && (
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-3 flex-wrap">
            {metrics?.cpu_used_percent != null && (
              <MiniBar
                value={metrics.cpu_used_percent}
                label={`CPU ${Math.round(metrics.cpu_used_percent)}%`}
              />
            )}
            {metrics?.memory_used_percent != null && (
              <MiniBar
                value={metrics.memory_used_percent}
                label={`MEM ${Math.round(metrics.memory_used_percent)}%`}
              />
            )}
            {metrics?.disk_used_percent != null && (
              <MiniBar
                value={metrics.disk_used_percent}
                label={`DSK ${Math.round(metrics.disk_used_percent)}%`}
              />
            )}
          </div>
          {onQuickAction && (
            <div className="flex items-center gap-1 shrink-0">
              <button
                onClick={() => onQuickAction("restart")}
                className={quickActionButtonClass}
                style={{ transitionDuration: "var(--dur-fast)" }}
                aria-label="Restart device"
                title="Restart"
              >
                <RotateCw size={14} />
              </button>
              <button
                onClick={() => onQuickAction("shutdown")}
                className={quickActionButtonClass}
                style={{ transitionDuration: "var(--dur-fast)" }}
                aria-label="Shutdown device"
                title="Shutdown"
              >
                <Power size={14} />
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

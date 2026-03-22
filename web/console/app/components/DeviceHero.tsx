"use client";

import { Badge } from "./ui/Badge";
import { Card } from "./ui/Card";
import { RingGauge } from "./ui/RingGauge";
import { FreshnessDisplay } from "./FreshnessDisplay";
import type { Asset, TelemetryOverviewAsset } from "../console/models";
import { friendlySourceLabel, friendlyTypeLabel, sourceIcon } from "../console/taxonomy";

type DeviceHeroProps = {
  asset: Asset;
  telemetry: TelemetryOverviewAsset | null;
  groupName: string;
  freshnessStatus: "ok" | "pending" | "bad";
  agentConnected: boolean;
};

type MetricsShape = TelemetryOverviewAsset["metrics"];

function metadataFallbackMetrics(meta?: Record<string, string>): Partial<MetricsShape> | undefined {
  if (!meta) return undefined;
  const result: Partial<MetricsShape> = {};

  // CPU
  const cpu = meta.cpu_used_percent ?? meta.cpu_percent;
  if (cpu && Number.isFinite(Number(cpu))) result.cpu_used_percent = Number(cpu);

  // Memory — try percentage first, then compute from total/available
  const memPct = meta.memory_used_percent ?? meta.memory_percent;
  if (memPct && Number.isFinite(Number(memPct))) {
    result.memory_used_percent = Number(memPct);
  } else {
    const total = Number(meta.physmem ?? meta.memory_total_bytes ?? "0");
    const avail = Number(meta.memory_available_bytes ?? "0");
    if (total > 0 && avail > 0) {
      result.memory_used_percent = ((total - avail) / total) * 100;
    }
  }

  // Disk
  const diskPct = meta.disk_used_percent ?? meta.disk_percent;
  if (diskPct && Number.isFinite(Number(diskPct))) result.disk_used_percent = Number(diskPct);

  // Temperature
  const temp = meta.temperature_celsius ?? meta.temp_celsius;
  if (temp && Number.isFinite(Number(temp))) result.temperature_celsius = Number(temp);

  return Object.keys(result).length > 0 ? result : undefined;
}

function titleCaseToken(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return "";
  return trimmed.charAt(0).toUpperCase() + trimmed.slice(1);
}

export function DeviceHero({ asset, telemetry, groupName, freshnessStatus, agentConnected }: DeviceHeroProps) {
  const metrics = telemetry?.metrics ?? metadataFallbackMetrics(asset.metadata);
  const os = asset.metadata?.os_pretty_name || asset.metadata?.os_name || "";
  const platform = asset.platform || "";
  const source = asset.source || "";
  const sourceLabel = source ? friendlySourceLabel(source) : "";
  const resourceKind = (asset.resource_kind || asset.type || "").trim();
  const resourceClass = (asset.resource_class || "").trim();

  return (
    <Card className="mb-4 space-y-3">
      {/* Row 1: Identity */}
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div className="flex items-center gap-2.5 min-w-0">
          <Badge
            status={freshnessStatus === "ok" ? "online" : freshnessStatus === "pending" ? "stale" : "offline"}
            dot
          />
          {(() => { const Icon = sourceIcon(asset.source); return <Icon size={16} className="text-[var(--muted)] shrink-0" />; })()}
          <h1 className="text-lg font-semibold text-[var(--text)] truncate">{asset.name}</h1>
          {agentConnected ? (
            <span className="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded-lg bg-emerald-500/10 text-emerald-500">
              <span className="inline-block h-1.5 w-1.5 rounded-full bg-emerald-500" />
              Agent Connected
            </span>
          ) : (
            <span className="text-[10px] px-1.5 py-0.5 rounded-lg border border-[var(--line)] text-[var(--muted)]">
              Agent Not Connected
            </span>
          )}
        </div>
        <FreshnessDisplay lastSeenAt={asset.last_seen_at} />
      </div>

      {/* Row 2: Context tags */}
      <div className="flex items-center gap-1.5 flex-wrap text-[10px]">
        {os && (
          <span className="px-1.5 py-0.5 rounded-lg border border-[var(--line)] text-[var(--muted)]">{os}</span>
        )}
        {platform && (
          <span className="px-1.5 py-0.5 rounded-lg border border-[var(--line)] text-[var(--muted)]">{platform}</span>
        )}
        {sourceLabel && (
          <span className="px-1.5 py-0.5 rounded-lg border border-[var(--line)] text-[var(--muted)]">{sourceLabel}</span>
        )}
        {resourceKind && (
          <span className="px-1.5 py-0.5 rounded-lg border border-[var(--line)] text-[var(--muted)]">{friendlyTypeLabel(resourceKind)}</span>
        )}
        {resourceClass && (
          <span className="px-1.5 py-0.5 rounded-lg border border-[var(--line)] text-[var(--muted)]">{titleCaseToken(resourceClass)}</span>
        )}
        {groupName && (
          <span className="px-1.5 py-0.5 rounded-lg border border-[var(--line)] text-[var(--muted)]">{groupName}</span>
        )}
      </div>

      {/* Row 3: Ring gauges (hidden when all data is missing) */}
      {(metrics?.cpu_used_percent != null || metrics?.memory_used_percent != null ||
        metrics?.disk_used_percent != null || metrics?.temperature_celsius != null) && (
        <div className="flex items-center justify-center gap-6 sm:gap-8 pt-1 flex-wrap">
          {metrics?.cpu_used_percent != null && <RingGauge label="CPU" value={metrics.cpu_used_percent} />}
          {metrics?.memory_used_percent != null && <RingGauge label="Memory" value={metrics.memory_used_percent} />}
          {metrics?.disk_used_percent != null && <RingGauge label="Disk" value={metrics.disk_used_percent} />}
          {metrics?.temperature_celsius != null && <RingGauge label="Temp" value={metrics.temperature_celsius} max={100} unit="C" />}
        </div>
      )}
    </Card>
  );
}

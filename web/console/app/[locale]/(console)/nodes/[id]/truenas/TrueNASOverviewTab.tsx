"use client";

import { Badge } from "../../../../../components/ui/Badge";
import { Card } from "../../../../../components/ui/Card";
import { formatBytes } from "../truenasTabModel";
import { useTrueNASOverview } from "./useTrueNASData";

type Props = {
  assetId: string;
};

function StatTile({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="rounded-lg border border-[var(--line)] bg-[var(--panel-glass)] px-4 py-3">
      <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">{label}</p>
      <p className="mt-1 text-sm font-medium text-[var(--text)] break-all">{value || "--"}</p>
      {hint ? <p className="mt-1 text-[11px] text-[var(--muted)]">{hint}</p> : null}
    </div>
  );
}

function alertLevelBadge(level: string): "ok" | "pending" | "bad" {
  const l = level.toLowerCase();
  if (l.includes("crit") || l.includes("error")) return "bad";
  if (l.includes("warn")) return "pending";
  return "ok";
}

export function TrueNASOverviewTab({ assetId }: Props) {
  const { data, loading, error } = useTrueNASOverview(assetId);

  if (loading && !data) {
    return (
      <Card>
        <p className="text-sm text-[var(--muted)]">Loading overview…</p>
      </Card>
    );
  }

  if (error && !data) {
    return (
      <Card>
        <p className="text-sm text-[var(--bad)]">{error}</p>
      </Card>
    );
  }

  if (!data) {
    return (
      <Card>
        <p className="text-sm text-[var(--muted)]">No overview data available.</p>
      </Card>
    );
  }

  const usedBytes = data.storage_used_bytes ?? 0;
  const totalBytes = data.storage_total_bytes ?? 0;
  const usagePct = totalBytes > 0 ? Math.round((usedBytes / totalBytes) * 100) : 0;

  const services = data.services ?? [];
  const runningServices = services.filter((s) => s.running);
  const stoppedServices = services.filter((s) => !s.running);
  const autoStartStopped = stoppedServices.filter((s) => s.enabled);

  const alerts = data.alerts ?? [];
  const criticalAlerts = alerts.filter((a) => alertLevelBadge(a.level) === "bad");
  const warnAlerts = alerts.filter((a) => alertLevelBadge(a.level) === "pending");
  const infoAlerts = alerts.filter(
    (a) => alertLevelBadge(a.level) !== "bad" && alertLevelBadge(a.level) !== "pending",
  );

  return (
    <div className="space-y-4">
      <Card>
        <h2 className="mb-3 text-sm font-medium text-[var(--text)]">System Info</h2>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-4">
          <StatTile label="Hostname" value={data.hostname ?? "--"} />
          <StatTile label="Version" value={data.version ?? "--"} />
          <StatTile label="Model" value={data.model ?? "--"} />
          <StatTile label="Uptime" value={data.uptime ?? "--"} />
          <StatTile label="CPU Cores" value={data.cpu_cores != null ? String(data.cpu_cores) : "--"} />
          <StatTile
            label="Memory"
            value={data.memory_bytes != null ? formatBytes(data.memory_bytes) : "--"}
          />
          <StatTile
            label="ECC Memory"
            value={data.ecc_enabled != null ? (data.ecc_enabled ? "Enabled" : "Disabled") : "--"}
          />
        </div>
      </Card>

      {totalBytes > 0 ? (
        <Card>
          <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Storage</h2>
          <div className="flex items-center justify-between text-xs text-[var(--muted)] mb-1">
            <span>{formatBytes(usedBytes)} used</span>
            <span>{formatBytes(totalBytes)} total · {usagePct}%</span>
          </div>
          <div className="h-2 rounded-full bg-[var(--surface)] overflow-hidden">
            <div
              className={`h-full rounded-full transition-all ${
                usagePct >= 90
                  ? "bg-[var(--bad)]"
                  : usagePct >= 75
                    ? "bg-[var(--warn)]"
                    : "bg-[var(--ok)]"
              }`}
              style={{ width: `${usagePct}%` }}
            />
          </div>
          <div className="mt-2 grid grid-cols-2 gap-3 sm:grid-cols-3">
            <StatTile label="Used" value={formatBytes(usedBytes)} />
            <StatTile label="Free" value={formatBytes(totalBytes - usedBytes)} />
            <StatTile label="Total" value={formatBytes(totalBytes)} />
          </div>
        </Card>
      ) : null}

      {services.length > 0 ? (
        <Card>
          <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Service Health</h2>
          <div className="flex items-center gap-4 text-sm mb-3">
            <span className="text-[var(--ok)]">{runningServices.length} running</span>
            <span className="text-[var(--muted)]">{stoppedServices.length} stopped</span>
          </div>
          {autoStartStopped.length > 0 ? (
            <div className="rounded-md bg-[var(--warn-surface,var(--surface))] border border-[var(--warn)] p-3 mb-3">
              <p className="text-xs font-medium text-[var(--warn)] mb-1">
                Auto-start services that are stopped
              </p>
              <ul className="space-y-0.5">
                {autoStartStopped.map((s) => (
                  <li key={s.name} className="text-xs text-[var(--text)]">{s.name}</li>
                ))}
              </ul>
            </div>
          ) : null}
        </Card>
      ) : null}

      {alerts.length > 0 ? (
        <Card>
          <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Alerts</h2>
          <div className="flex items-center gap-4 text-sm mb-3">
            {criticalAlerts.length > 0 ? (
              <span className="text-[var(--bad)]">{criticalAlerts.length} critical</span>
            ) : null}
            {warnAlerts.length > 0 ? (
              <span className="text-[var(--warn)]">{warnAlerts.length} warning</span>
            ) : null}
            {infoAlerts.length > 0 ? (
              <span className="text-[var(--muted)]">{infoAlerts.length} info</span>
            ) : null}
          </div>
          <ul className="divide-y divide-[var(--line)]">
            {alerts.slice(0, 20).map((alert, idx) => (
              <li key={idx} className="py-2 flex items-start gap-3">
                <Badge status={alertLevelBadge(alert.level)} size="sm" />
                <span className="text-xs text-[var(--text)]">{alert.message}</span>
              </li>
            ))}
          </ul>
        </Card>
      ) : null}
    </div>
  );
}

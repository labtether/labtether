import type { WebServiceHealthPoint, WebServiceHealthSummary } from "../../../hooks/useWebServices";
import { formatResponseTime } from "./servicesPageHelpers";

interface ServiceHealthDetailsProps {
  health?: WebServiceHealthSummary;
  currentStatus: string;
}

interface DerivedHealthMetrics {
  averageResponseMs?: number;
  currentStreakStart?: string;
  currentStatus: string;
  downPeriods: number;
  lastDownAt?: string;
  maxResponseMs?: number;
  recentChecks: WebServiceHealthPoint[];
  transitions: number;
  unknownPeriods: number;
}

function normalizeStatus(status: string): "up" | "down" | "unknown" {
  const normalized = status.trim().toLowerCase();
  if (normalized === "up" || normalized === "down") {
    return normalized;
  }
  return "unknown";
}

function statusLabel(status: string): string {
  switch (normalizeStatus(status)) {
    case "up":
      return "Up";
    case "down":
      return "Down";
    default:
      return "Unknown";
  }
}

function statusPanelClasses(status: string): string {
  switch (normalizeStatus(status)) {
    case "up":
      return "border-[var(--ok)]/25 bg-[var(--ok)]/10 text-[var(--ok)]";
    case "down":
      return "border-[var(--bad)]/30 bg-[var(--bad)]/10 text-[var(--bad)]";
    default:
      return "border-[var(--line)] bg-[var(--surface)]/70 text-[var(--muted)]";
  }
}

function historyPointClasses(status: string): string {
  switch (normalizeStatus(status)) {
    case "up":
      return "border-emerald-400/30 bg-emerald-400/15";
    case "down":
      return "border-rose-400/35 bg-rose-400/20";
    default:
      return "border-zinc-400/30 bg-zinc-400/15";
  }
}

function historyPointFill(status: string): string {
  switch (normalizeStatus(status)) {
    case "up":
      return "bg-emerald-400";
    case "down":
      return "bg-rose-400";
    default:
      return "bg-zinc-400";
  }
}

function formatHealthTimestamp(raw?: string): string {
  if (!raw) {
    return "n/a";
  }
  const parsed = new Date(raw);
  if (Number.isNaN(parsed.getTime())) {
    return raw;
  }
  return parsed.toLocaleString();
}

function formatRelativeHealthTime(raw?: string): string {
  if (!raw) {
    return "n/a";
  }
  const parsed = new Date(raw);
  if (Number.isNaN(parsed.getTime())) {
    return raw;
  }

  const deltaMs = Date.now() - parsed.getTime();
  const absolute = Math.abs(deltaMs);
  const future = deltaMs < 0;

  let value = Math.round(absolute / 1000);
  let unit = "sec";
  if (absolute >= 60 * 1000) {
    value = Math.round(absolute / (60 * 1000));
    unit = "min";
  }
  if (absolute >= 60 * 60 * 1000) {
    value = Math.round(absolute / (60 * 60 * 1000));
    unit = "hr";
  }
  if (absolute >= 24 * 60 * 60 * 1000) {
    value = Math.round(absolute / (24 * 60 * 60 * 1000));
    unit = "day";
  }

  const suffix = value === 1 ? "" : "s";
  return future ? `in ${value} ${unit}${suffix}` : `${value} ${unit}${suffix} ago`;
}

function describePeriods(count: number, singular: string): string {
  if (count === 1) {
    return `1 ${singular}`;
  }
  return `${count} ${singular}s`;
}

function deriveHealthMetrics(
  health: WebServiceHealthSummary,
  fallbackStatus: string,
): DerivedHealthMetrics {
  const recent = health.recent ?? [];
  const recentChecks = recent.slice(-6).reverse();
  let currentStatus = normalizeStatus(fallbackStatus);
  let currentStreakStart = health.last_change_at || health.last_checked_at;
  let transitions = 0;
  let downPeriods = 0;
  let unknownPeriods = 0;
  let lastDownAt = "";
  let responseTotal = 0;
  let responseCount = 0;
  let maxResponseMs = 0;

  if (recent.length > 0) {
    currentStatus = normalizeStatus(recent[recent.length - 1].status);
    currentStreakStart = recent[recent.length - 1].at;

    let previousStatus = "";
    for (let index = 0; index < recent.length; index += 1) {
      const point = recent[index];
      const status = normalizeStatus(point.status);
      if (index > 0 && status !== previousStatus) {
        transitions += 1;
      }
      if (status !== previousStatus) {
        if (status === "down") {
          downPeriods += 1;
          lastDownAt = point.at;
        } else if (status === "unknown") {
          unknownPeriods += 1;
        }
      }
      if (typeof point.response_ms === "number" && point.response_ms > 0) {
        responseTotal += point.response_ms;
        responseCount += 1;
        maxResponseMs = Math.max(maxResponseMs, point.response_ms);
      }
      previousStatus = status;
    }

    for (let index = recent.length - 2; index >= 0; index -= 1) {
      const point = recent[index];
      if (normalizeStatus(point.status) !== currentStatus) {
        break;
      }
      currentStreakStart = point.at;
    }
  }

  return {
    averageResponseMs: responseCount > 0 ? Math.round(responseTotal / responseCount) : undefined,
    currentStreakStart: currentStreakStart || undefined,
    currentStatus,
    downPeriods,
    lastDownAt: lastDownAt || undefined,
    maxResponseMs: maxResponseMs > 0 ? maxResponseMs : undefined,
    recentChecks,
    transitions,
    unknownPeriods,
  };
}

function HealthMetric({
  label,
  value,
  detail,
  tone = "default",
}: {
  detail?: string;
  label: string;
  tone?: "default" | "up" | "down" | "unknown";
  value: string;
}) {
  const toneClasses =
    tone === "up"
      ? "border-[var(--ok)]/20 bg-[var(--ok)]/8"
      : tone === "down"
        ? "border-[var(--bad)]/20 bg-[var(--bad)]/8"
        : tone === "unknown"
          ? "border-[var(--line)] bg-[var(--surface)]/70"
          : "border-[var(--line)] bg-[var(--surface)]/60";

  return (
    <div className={`rounded-lg border px-2.5 py-2 ${toneClasses}`}>
      <div className="text-[10px] font-medium uppercase tracking-[0.14em] text-[var(--muted)]">
        {label}
      </div>
      <div className="mt-1 text-[13px] font-semibold text-[var(--text)]">
        {value}
      </div>
      {detail && (
        <div className="mt-0.5 text-[10px] text-[var(--muted)]">
          {detail}
        </div>
      )}
    </div>
  );
}

export function ServiceHealthDetails({
  health,
  currentStatus,
}: ServiceHealthDetailsProps) {
  if (!health || health.checks <= 0) {
    return null;
  }

  const metrics = deriveHealthMetrics(health, currentStatus);
  const recent = health.recent ?? [];
  const historyStart = recent[0]?.at;
  const historyEnd = recent[recent.length - 1]?.at ?? health.last_checked_at;
  const maxResponseMs = Math.max(
    ...recent.map((point) => (typeof point.response_ms === "number" ? point.response_ms : 0)),
    1,
  );
  const currentTone =
    metrics.currentStatus === "up"
      ? "up"
      : metrics.currentStatus === "down"
        ? "down"
        : "unknown";

  return (
    <div
      className="mt-2 rounded-xl border border-[var(--line)] bg-[var(--surface)]/45 p-2.5"
      aria-label="Health History"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <div className="text-[11px] font-medium text-[var(--text)]">Health History</div>
          <div className="mt-0.5 text-[10px] text-[var(--muted)]">
            {health.window} rolling window · {health.checks} checks recorded
          </div>
        </div>
        {health.last_checked_at && (
          <div className="rounded-full border border-[var(--line)] bg-[var(--panel)]/70 px-2 py-1 text-[10px] text-[var(--muted)]">
            Last check {formatRelativeHealthTime(health.last_checked_at)}
          </div>
        )}
      </div>

      <div className="mt-2 grid grid-cols-2 gap-2 lg:grid-cols-4">
        <HealthMetric
          label="Uptime"
          value={`${health.uptime_percent.toFixed(1)}%`}
          detail={`${health.up_checks}/${health.checks} up`}
          tone={health.uptime_percent >= 99 ? "up" : health.uptime_percent < 95 ? "down" : "default"}
        />
        <HealthMetric
          label="Current State"
          value={statusLabel(metrics.currentStatus)}
          detail={metrics.currentStreakStart ? `Since ${formatHealthTimestamp(metrics.currentStreakStart)}` : undefined}
          tone={currentTone}
        />
        <HealthMetric
          label="Transitions"
          value={describePeriods(metrics.transitions, "change")}
          detail={health.last_change_at ? `Last change ${formatRelativeHealthTime(health.last_change_at)}` : "Stable in current window"}
        />
        <HealthMetric
          label="Latency"
          value={formatResponseTime(metrics.averageResponseMs ?? 0) || "n/a"}
          detail={metrics.maxResponseMs ? `Peak ${formatResponseTime(metrics.maxResponseMs)}` : "No response data"}
        />
      </div>

      {recent.length > 0 && (
        <div className="mt-2.5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="text-[10px] font-medium uppercase tracking-[0.12em] text-[var(--muted)]">
              Availability Timeline
            </div>
            <div className="text-[10px] text-[var(--muted)]">
              {historyStart ? formatHealthTimestamp(historyStart) : "n/a"} → {formatHealthTimestamp(historyEnd)}
            </div>
          </div>
          <div
            className="mt-1.5 flex items-end gap-1"
            aria-label={`Availability timeline with ${recent.length} samples`}
          >
            {recent.map((point, index) => {
              const responseMs = typeof point.response_ms === "number" ? point.response_ms : 0;
              const barHeight = responseMs > 0 ? Math.max(4, Math.round((responseMs / maxResponseMs) * 22)) : 4;
              return (
                <div
                  key={`${point.at}-${index}`}
                  className={`flex min-w-[8px] flex-1 items-end rounded-md border px-[2px] py-[2px] ${historyPointClasses(point.status)}`}
                  title={`${statusLabel(point.status)} · ${formatResponseTime(responseMs) || "No latency"} · ${formatHealthTimestamp(point.at)}`}
                >
                  <div
                    className={`w-full rounded-sm ${historyPointFill(point.status)}`}
                    style={{ height: `${barHeight}px` }}
                  />
                </div>
              );
            })}
          </div>
          <div className="mt-1.5 flex flex-wrap gap-1.5 text-[10px] text-[var(--muted)]">
            <span className="rounded-full border border-[var(--line)] bg-[var(--panel)]/70 px-2 py-0.5">
              {describePeriods(metrics.downPeriods, "down period")}
            </span>
            {metrics.unknownPeriods > 0 && (
              <span className="rounded-full border border-[var(--line)] bg-[var(--panel)]/70 px-2 py-0.5">
                {describePeriods(metrics.unknownPeriods, "unknown period")}
              </span>
            )}
            {metrics.lastDownAt && (
              <span className="rounded-full border border-[var(--line)] bg-[var(--panel)]/70 px-2 py-0.5">
                Last outage {formatRelativeHealthTime(metrics.lastDownAt)}
              </span>
            )}
          </div>
        </div>
      )}

      {metrics.recentChecks.length > 0 && (
        <div className="mt-2.5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="text-[10px] font-medium uppercase tracking-[0.12em] text-[var(--muted)]">
              Recent Checks
            </div>
            <div className="text-[10px] text-[var(--muted)]">
              {metrics.recentChecks.length} latest samples
            </div>
          </div>
          <div className="mt-1.5 space-y-1.5">
            {metrics.recentChecks.map((point, index) => (
              <div
                key={`${point.at}-recent-${index}`}
                className="flex items-center justify-between gap-2 rounded-lg border border-[var(--line)] bg-[var(--panel)]/55 px-2 py-1.5"
              >
                <div className="flex min-w-0 items-center gap-2">
                  <span
                    className={`inline-flex rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.12em] ${statusPanelClasses(point.status)}`}
                  >
                    {statusLabel(point.status)}
                  </span>
                  <span className="truncate text-[10px] text-[var(--muted)]">
                    {formatHealthTimestamp(point.at)}
                  </span>
                </div>
                <span className="shrink-0 text-[10px] font-mono text-[var(--muted)]">
                  {formatResponseTime(point.response_ms ?? 0) || "n/a"}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

"use client";

import { Card } from "../../../../components/ui/Card";
import { SegmentedTabs } from "../../../../components/ui/SegmentedTabs";
import { telemetryWindows } from "../../../../console/models";
import type { LogEvent, TelemetryWindow } from "../../../../console/models";

type NodeLogsTabCardProps = {
  logsLoading: boolean;
  logsError: string | null;
  logEvents: LogEvent[];
  logLevelFilter: string;
  onLogLevelFilterChange: (value: string) => void;
  logMode: "stored" | "journal";
  onLogModeChange: (value: "stored" | "journal") => void;
  supportsLogQuery: boolean;
  logQueryModeLabel: string;
  logWindow: TelemetryWindow;
  onLogWindowChange: (value: TelemetryWindow) => void;
  journalSince: string;
  onJournalSinceChange: (value: string) => void;
  journalUntil: string;
  onJournalUntilChange: (value: string) => void;
  journalUnit: string;
  onJournalUnitChange: (value: string) => void;
  journalPriority: string;
  onJournalPriorityChange: (value: string) => void;
  journalQuery: string;
  onJournalQueryChange: (value: string) => void;
  journalLiveTail: boolean;
  onJournalLiveTailChange: (value: boolean) => void;
  onRefresh: () => void;
};

const journalUnitPresets = [
  { label: "All", unit: "" },
  { label: "SSH", unit: "ssh.service" },
  { label: "Docker", unit: "docker.service" },
  { label: "Tailscale", unit: "tailscaled.service" },
  { label: "Network", unit: "NetworkManager.service" },
  { label: "LabTether", unit: "labtether-agent.service" },
];

export function NodeLogsTabCard({
  logsLoading,
  logsError,
  logEvents,
  logLevelFilter,
  onLogLevelFilterChange,
  logMode,
  onLogModeChange,
  supportsLogQuery,
  logQueryModeLabel,
  logWindow,
  onLogWindowChange,
  journalSince,
  onJournalSinceChange,
  journalUntil,
  onJournalUntilChange,
  journalUnit,
  onJournalUnitChange,
  journalPriority,
  onJournalPriorityChange,
  journalQuery,
  onJournalQueryChange,
  journalLiveTail,
  onJournalLiveTailChange,
  onRefresh,
}: NodeLogsTabCardProps) {
  const filtered = logEvents.filter((event) => {
    if (logLevelFilter === "all") {
      return true;
    }
    return normalizeLogLevel(event.level) === logLevelFilter;
  });

  return (
    <Card className="mb-4">
      <div className="flex items-center justify-between gap-3 mb-3 flex-wrap">
        <h2 className="text-sm font-medium text-[var(--text)]">Device Logs</h2>
        <div className="flex items-center gap-2 flex-wrap">
          <SegmentedTabs
            size="sm"
            value={logMode}
            options={supportsLogQuery
              ? [
                  { id: "stored", label: "Stored" },
                  { id: "journal", label: logQueryModeLabel },
                ]
              : [{ id: "stored", label: "Stored" }]}
            onChange={(value) => onLogModeChange(value as "stored" | "journal")}
          />
          <button
            onClick={onRefresh}
            disabled={logsLoading}
            className="text-xs px-2 py-1 rounded border border-[var(--line)] text-[var(--muted)] disabled:opacity-50"
          >
            {logsLoading ? "Loading..." : "Refresh"}
          </button>
          {logMode === "journal" && supportsLogQuery ? (
            <button
              onClick={() => onJournalLiveTailChange(!journalLiveTail)}
              className={`text-xs px-2 py-1 rounded border border-[var(--line)] ${journalLiveTail ? "text-[var(--warn)] bg-[var(--warn-glow)]" : "text-[var(--muted)]"}`}
            >
              {journalLiveTail ? "Pause Tail" : "Live Tail"}
            </button>
          ) : null}
        </div>
      </div>

      <div className="mb-3 flex items-center gap-2 flex-wrap">
        {logMode === "stored" || !supportsLogQuery ? (
          <select
            value={logWindow}
            onChange={(event) => onLogWindowChange(event.target.value as TelemetryWindow)}
            className="text-xs px-2 py-1 rounded border border-[var(--line)] bg-[var(--surface)] text-[var(--text)]"
          >
            {telemetryWindows.map((window) => (
              <option key={window} value={window}>{window}</option>
            ))}
          </select>
        ) : (
          <>
            <input
              type="text"
              value={journalSince}
              onChange={(event) => onJournalSinceChange(event.target.value)}
              placeholder="Since (e.g. 1h ago)"
              className="text-xs px-2 py-1 rounded border border-[var(--line)] bg-transparent text-[var(--text)] min-w-[150px]"
            />
            <input
              type="text"
              value={journalUntil}
              onChange={(event) => onJournalUntilChange(event.target.value)}
              placeholder="Until (optional)"
              className="text-xs px-2 py-1 rounded border border-[var(--line)] bg-transparent text-[var(--text)] min-w-[120px]"
            />
            <input
              type="text"
              value={journalUnit}
              onChange={(event) => onJournalUnitChange(event.target.value)}
              placeholder="Unit (e.g. ssh.service)"
              className="text-xs px-2 py-1 rounded border border-[var(--line)] bg-transparent text-[var(--text)] min-w-[170px]"
            />
            <div className="flex items-center gap-1 flex-wrap">
              {journalUnitPresets.map((preset) => (
                <button
                  key={preset.label}
                  onClick={() => onJournalUnitChange(preset.unit)}
                  className={`text-[10px] px-2 py-1 rounded border border-[var(--line)] ${journalUnit.trim() === preset.unit ? "text-[var(--text)] bg-[var(--surface)]" : "text-[var(--muted)]"}`}
                >
                  {preset.label}
                </button>
              ))}
            </div>
            <select
              value={journalPriority}
              onChange={(event) => onJournalPriorityChange(event.target.value)}
              className="text-xs px-2 py-1 rounded border border-[var(--line)] bg-[var(--surface)] text-[var(--text)] min-w-[120px]"
            >
              <option value="all">Priority: all</option>
              <option value="debug">debug</option>
              <option value="info">info</option>
              <option value="notice">notice</option>
              <option value="warning">warning</option>
              <option value="err">err</option>
              <option value="crit">crit</option>
              <option value="alert">alert</option>
              <option value="emerg">emerg</option>
            </select>
            <input
              type="text"
              value={journalQuery}
              onChange={(event) => onJournalQueryChange(event.target.value)}
              placeholder="Search message text"
              className="text-xs px-2 py-1 rounded border border-[var(--line)] bg-transparent text-[var(--text)] min-w-[180px]"
            />
            {journalLiveTail ? (
              <span className="text-[11px] text-[var(--warn)]">Live tail: polling journal every 2s</span>
            ) : null}
          </>
        )}

        {!logsLoading && logEvents.length > 0 ? (
          <SegmentedTabs
            size="sm"
            value={logLevelFilter}
            options={(["all", "error", "warn", "info"] as const).map((level) => {
              const count = level === "all"
                ? logEvents.length
                : logEvents.filter((event) => normalizeLogLevel(event.level) === level).length;
              const levelLabel = level === "all"
                ? "All"
                : level === "error"
                  ? "Error"
                  : level === "warn"
                    ? "Warning"
                    : "Info";
              return { id: level, label: `${levelLabel} (${count})` };
            })}
            onChange={onLogLevelFilterChange}
          />
        ) : null}
      </div>
      {logsLoading ? (
        <p className="text-sm text-[var(--muted)]">Loading logs...</p>
      ) : logsError ? (
        <p className="text-sm text-[var(--bad)]">{logsError}</p>
      ) : filtered.length > 0 ? (
        <ul className="divide-y divide-[var(--line)]">
          {filtered
            .map((event) => {
              const normalizedLevel = normalizeLogLevel(event.level);
              const borderColor = normalizedLevel === "error"
                ? "border-l-[var(--bad)]"
                : normalizedLevel === "warn"
                  ? "border-l-[var(--warn)]"
                  : "border-l-[var(--ok)]";
              const diffMs = Date.now() - new Date(event.timestamp).getTime();
              const relativeTime = diffMs < 60_000
                ? `${Math.floor(diffMs / 1000)}s ago`
                : diffMs < 3_600_000
                  ? `${Math.floor(diffMs / 60_000)}m ago`
                  : `${Math.floor(diffMs / 3_600_000)}h ago`;
              return (
                <li key={event.id} className={`flex items-center gap-3 py-2 pl-2 border-l-2 ${borderColor}`}>
                  <span className="text-xs font-medium text-[var(--text)] shrink-0">{event.source}</span>
                  <span className="text-xs text-[var(--text)] truncate flex-1">{event.message}</span>
                  <span
                    className="text-xs text-[var(--muted)] shrink-0"
                    title={new Date(event.timestamp).toLocaleString()}
                  >
                    {relativeTime}
                  </span>
                </li>
              );
            })}
        </ul>
      ) : (
        <div className="flex flex-col items-center justify-center py-12 gap-2">
          <p className="text-sm font-medium text-[var(--text)]">No logs yet</p>
          <p className="text-xs text-[var(--muted)] text-center max-w-sm">No logs from this device in the current time range.</p>
        </div>
      )}
    </Card>
  );
}

function normalizeLogLevel(level: string): string {
  const normalized = level.toLowerCase();
  if (normalized === "warning") {
    return "warn";
  }
  if (normalized === "err" || normalized === "crit" || normalized === "critical" || normalized === "alert" || normalized === "emerg") {
    return "error";
  }
  if (normalized === "notice" || normalized === "debug") {
    return "info";
  }
  return normalized;
}

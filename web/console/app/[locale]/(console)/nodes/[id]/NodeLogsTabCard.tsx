"use client";

import { RefreshCw } from "lucide-react";
import { useTranslations } from "next-intl";
import type { CSSProperties } from "react";
import { Button } from "../../../../components/ui/Button";
import { telemetryWindows } from "../../../../console/models";
import type { LogEvent, TelemetryWindow } from "../../../../console/models";
import { cleanLoadError, formatLogTime, fullTime, normalizeLogLevel } from "./logsViewParts";

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

/** Units offered while typing in the journal's Unit field (the field takes any unit). */
const journalUnitPresets = [
  "ssh.service",
  "docker.service",
  "tailscaled.service",
  "NetworkManager.service",
  "labtether-agent.service",
];
const journalPriorities = ["emerg", "alert", "crit", "err", "warning", "notice", "info", "debug"] as const;
const levelFilters = ["all", "error", "warn", "info"] as const;
const COLS = { "--cols": "88px 48px minmax(0, 160px) minmax(0, 1fr)" } as CSSProperties;

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
  const t = useTranslations("devices.logsTab");
  const journal = logMode === "journal" && supportsLogQuery;
  const filtered = logEvents.filter((event) => logLevelFilter === "all" || normalizeLogLevel(event.level) === logLevelFilter);
  const detail = cleanLoadError(logsError);

  return (
    <div className="dst-logs">
      <div className="dv-bar">
        {supportsLogQuery ? (
          <div className="dv-seg" role="group" aria-label={t("source")}>
            <button type="button" aria-pressed={logMode === "stored"} onClick={() => onLogModeChange("stored")}>{t("stored")}</button>
            <button type="button" aria-pressed={logMode === "journal"} onClick={() => onLogModeChange("journal")}>{logQueryModeLabel}</button>
          </div>
        ) : null}
        {!journal ? (
          <select
            className="dv-input dst-select"
            aria-label={t("window")}
            value={logWindow}
            onChange={(event) => onLogWindowChange(event.target.value as TelemetryWindow)}
          >
            {telemetryWindows.map((window) => (
              <option key={window} value={window}>{t(`windows.${window}`)}</option>
            ))}
          </select>
        ) : null}
        {logEvents.length > 0 ? (
          <div className="dv-seg" role="group" aria-label={t("level")}>
            {levelFilters.map((level) => (
              <button key={level} type="button" aria-pressed={logLevelFilter === level} onClick={() => onLogLevelFilterChange(level)}>
                {t(`levels.${level}`)}
              </button>
            ))}
          </div>
        ) : null}
        <span className="dv-sp" />
        {journal ? (
          <Button
            size="sm"
            variant="ghost"
            aria-pressed={journalLiveTail}
            className={journalLiveTail ? "dst-live on" : "dst-live"}
            onClick={() => onJournalLiveTailChange(!journalLiveTail)}
          >
            {journalLiveTail ? <span className="dv-dot busy" aria-hidden="true" /> : null}
            {t("live")}
          </Button>
        ) : null}
        <button type="button" className="dst-icon" onClick={onRefresh} disabled={logsLoading} aria-label={t("refresh")} title={t("refresh")}>
          <RefreshCw aria-hidden="true" />
        </button>
      </div>

      {journal ? (
        <div className="dv-bar dst-journal">
          <label className="dst-inline">
            <span>{t("since")}</span>
            <input className="dv-input" value={journalSince} onChange={(e) => onJournalSinceChange(e.target.value)} placeholder={t("sincePlaceholder")} />
          </label>
          {!journalLiveTail ? (
            <label className="dst-inline">
              <span>{t("until")}</span>
              <input className="dv-input" value={journalUntil} onChange={(e) => onJournalUntilChange(e.target.value)} placeholder={t("untilPlaceholder")} />
            </label>
          ) : null}
          <label className="dst-inline">
            <span>{t("unit")}</span>
            <input className="dv-input dst-mono-in" list="dst-journal-units" value={journalUnit} onChange={(e) => onJournalUnitChange(e.target.value)} placeholder={t("unitPlaceholder")} spellCheck={false} />
          </label>
          <datalist id="dst-journal-units">
            {journalUnitPresets.map((unit) => <option key={unit} value={unit} />)}
          </datalist>
          <select className="dv-input dst-select" aria-label={t("priority")} value={journalPriority} onChange={(e) => onJournalPriorityChange(e.target.value)}>
            <option value="all">{t("priorities.all")}</option>
            {journalPriorities.map((priority) => <option key={priority} value={priority}>{t(`priorities.${priority}`)}</option>)}
          </select>
          <input
            className="dv-input dst-search"
            type="search"
            aria-label={t("search")}
            value={journalQuery}
            onChange={(e) => onJournalQueryChange(e.target.value)}
            placeholder={t("searchPlaceholder")}
          />
        </div>
      ) : null}

      <div
        className="dv-list dst-log-list"
        role={filtered.length > 0 && !logsError && !(logsLoading && logEvents.length === 0) ? "table" : undefined}
        aria-label={t("listLabel")}
        style={COLS}
      >
        {logsLoading && logEvents.length === 0 ? (
          <p className="dst-quiet">{t("loading")}</p>
        ) : logsError ? (
          <div className="dv-empty">
            <b>{t("loadError")}</b>
            {detail ? <span>{detail}</span> : null}
            <Button size="sm" className="dv-empty-act" onClick={onRefresh}>{t("retry")}</Button>
          </div>
        ) : filtered.length === 0 ? (
          <div className="dv-empty">
            <b>{t("empty")}</b>
            <span>{t("emptyHint")}</span>
          </div>
        ) : (
          <>
            <div className="dv-cols" role="row">
              <span role="columnheader">{t("columns.time")}</span>
              <span role="columnheader">{t("columns.level")}</span>
              <span role="columnheader">{t("columns.source")}</span>
              <span role="columnheader">{t("columns.message")}</span>
            </div>
            {filtered.map((event) => {
              const level = normalizeLogLevel(event.level);
              return (
                <div key={event.id} className="dv-row dst-ln" role="row">
                  <time className="dst-t" dateTime={event.timestamp} title={fullTime(event.timestamp)} role="cell">
                    {formatLogTime(event.timestamp)}
                  </time>
                  <span role="cell" className={`dst-lv ${level}`}>
                    {level === "error" ? t("levelWords.error") : level === "warn" ? t("levelWords.warn") : ""}
                  </span>
                  <span role="cell" className="dv-grey">{event.source}</span>
                  <span role="cell" className="dst-msg">{event.message}</span>
                </div>
              );
            })}
          </>
        )}
      </div>
    </div>
  );
}

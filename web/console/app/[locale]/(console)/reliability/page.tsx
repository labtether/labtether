"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { HeartPulse } from "lucide-react";
import { PageHeader } from "../../../components/PageHeader";
import { Card } from "../../../components/ui/Card";
import { EmptyState } from "../../../components/ui/EmptyState";
import { Button } from "../../../components/ui/Button";
import { Input } from "../../../components/ui/Input";
import { Select } from "../../../components/ui/Input";
import { Badge } from "../../../components/ui/Badge";
import { SegmentedTabs } from "../../../components/ui/SegmentedTabs";
import { SkeletonRow } from "../../../components/ui/Skeleton";
import { WorldMapSVG } from "../../../components/WorldMapSVG";
import { useSlowStatus } from "../../../contexts/StatusContext";
import { useReliability } from "../../../hooks/useReliability";
import { formatTimestamp } from "../../../console/formatters";
import { groupTimelineWindows } from "../../../console/models";

type ReliabilityTab = "scores" | "timeline" | "maintenance" | "map";

export default function ReliabilityPage() {
  const t = useTranslations('reliability');
  const status = useSlowStatus();
  const {
    groupRows,
    groupReliabilityRows,
    deadLetters,
    deadLetterAnalytics,
    selectedTimelineGroup,
    setSelectedTimelineGroup,
    groupTimelineWindow,
    setGroupTimelineWindow,
    groupTimeline,
    groupTimelineLoading,
    groupTimelineError,
    maintenanceWindows,
    maintenanceLoading,
    maintenanceMessage,
    maintenanceName,
    setMaintenanceName,
    maintenanceStart,
    setMaintenanceStart,
    maintenanceEnd,
    setMaintenanceEnd,
    maintenanceSuppressAlerts,
    setMaintenanceSuppressAlerts,
    maintenanceBlockActions,
    setMaintenanceBlockActions,
    maintenanceBlockUpdates,
    setMaintenanceBlockUpdates,
    maintenanceSaving,
    createMaintenanceWindow,
    deleteMaintenanceWindow
  } = useReliability();

  const [activeTab, setActiveTab] = useState<ReliabilityTab>("scores");
  const [timelineExpanded, setTimelineExpanded] = useState(false);

  return (
    <>
      <PageHeader title={t('title')} subtitle={t('subtitle')} />

      {/* Tab bar */}
      <Card className="flex items-center justify-between mb-4">
        <SegmentedTabs
          value={activeTab}
          options={(["scores", "timeline", "maintenance", "map"] as ReliabilityTab[]).map((tab) => ({
            id: tab,
            label: t(`tabs.${tab}`),
          }))}
          onChange={setActiveTab}
        />
      </Card>

      {/* Scores tab */}
      {activeTab === "scores" ? (
        <Card className="mb-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
            <div>
              <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-2">{t('scores.locationHealth')}</p>
              <ul className="divide-y divide-[var(--line)]">
                {groupReliabilityRows.map((entry) => (
                  <li key={entry.group.id} className="flex items-center justify-between gap-3 py-2.5">
                    <span className="text-sm font-medium text-[var(--text)]">
                      {entry.group.name}
                      {` / grade ${entry.grade}`}
                    </span>
                    <Badge status={entry.score >= 90 ? "ok" : entry.score >= 75 ? "pending" : "bad"} size="sm" />
                  </li>
                ))}
                {groupReliabilityRows.length === 0 ? (
                  <li>
                    <EmptyState icon={HeartPulse} title={t('scores.healthEmpty.title')} description={t('scores.healthEmpty.description')} />
                  </li>
                ) : null}
              </ul>
            </div>
            <div className="space-y-4 divide-y divide-[var(--line)]">
              <div>
                <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-2">{t('scores.failedJobs')}</p>
                <ul className="divide-y divide-[var(--line)]">
                  {deadLetters.slice(0, 8).map((event) => (
                    <li key={event.id} className="flex items-center justify-between gap-3 py-2.5">
                      <span className="text-sm font-medium text-[var(--text)]">{event.component || "worker"}</span>
                      <Badge status="bad" size="sm" />
                    </li>
                  ))}
                  {deadLetters.length === 0 ? <li className="py-2.5 text-xs text-[var(--muted)]">{t('scores.noFailedJobs')}</li> : null}
                </ul>
                <p className="text-xs text-[var(--muted)] mt-4">
                  {deadLetterAnalytics.rate_per_hour.toFixed(2)}/h ({deadLetterAnalytics.rate_per_day.toFixed(2)}/day)
                </p>
              </div>
              <div className="pt-4">
                <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-2">{t('scores.commonErrors')}</p>
                <ul className="divide-y divide-[var(--line)]">
                  {deadLetterAnalytics.top_error_classes.slice(0, 4).map((entry) => (
                    <li key={entry.key} className="flex items-center justify-between gap-3 py-2.5">
                      <span className="text-sm font-medium text-[var(--text)]">{entry.key}</span>
                      <Badge status="bad" size="sm" />
                    </li>
                  ))}
                  {deadLetterAnalytics.top_error_classes.length === 0 ? (
                    <li className="py-2.5 text-xs text-[var(--muted)]">{t('scores.noRecurringErrors')}</li>
                  ) : null}
                </ul>
              </div>
              <div className="pt-4">
                <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-2">{t('scores.failedJobsTrend')}</p>
                <ul className="divide-y divide-[var(--line)]">
                  {deadLetterAnalytics.trend.slice(-6).map((point) => (
                    <li key={point.start} className="flex items-center justify-between gap-3 py-2.5">
                      <span className="text-sm font-medium text-[var(--text)]">{new Date(point.start).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</span>
                      <div className="flex items-center gap-2">
                        <span className="text-xs font-mono text-[var(--muted)]">{point.count}</span>
                        <Badge status={point.count > 0 ? "bad" : "ok"} size="sm" />
                      </div>
                    </li>
                  ))}
                  {deadLetterAnalytics.trend.length === 0 ? <li className="py-2.5 text-xs text-[var(--muted)]">{t('scores.noTrendData')}</li> : null}
                </ul>
              </div>
              <div className="pt-4">
                <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-2">{t('scores.dataCleanup')}</p>
                {status?.summary.retentionError ? (
                  <p className="text-xs text-[var(--bad)]">{status.summary.retentionError}</p>
                ) : (
                  <p className="text-xs text-[var(--muted)]">{t('scores.retentionHealthy')}</p>
                )}
              </div>
            </div>
          </div>
        </Card>
      ) : null}

      {/* Timeline tab */}
      {activeTab === "timeline" ? (
        <Card className="mb-4">
          <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-3">{t('timeline.heading')}</p>
          <div className="flex flex-wrap items-end gap-4 mb-4">
            <div className="flex flex-col gap-1">
              <span className="text-xs text-[var(--muted)]">{t('timeline.group')}</span>
              <Select
                value={selectedTimelineGroup}
                onChange={(event) => setSelectedTimelineGroup(event.target.value)}
              >
                {groupRows.map((group) => (
                  <option key={group.id} value={group.id}>
                    {group.name}
                  </option>
                ))}
              </Select>
            </div>
            <div className="flex flex-col gap-1">
              <span className="text-xs text-[var(--muted)]">{t('timeline.range')}</span>
              <SegmentedTabs
                value={groupTimelineWindow}
                options={groupTimelineWindows.map((windowValue) => ({ id: windowValue, label: windowValue }))}
                onChange={setGroupTimelineWindow}
              />
            </div>
          </div>

          {groupTimeline ? (
            <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3 mb-4">
              <div className="text-center">
                <span className="text-xs text-[var(--muted)]">{t('timeline.errors')}</span>
                <strong className="text-lg font-medium text-[var(--text)] block">{groupTimeline.impact.error_events}</strong>
              </div>
              <div className="text-center">
                <span className="text-xs text-[var(--muted)]">{t('timeline.warnings')}</span>
                <strong className="text-lg font-medium text-[var(--text)] block">{groupTimeline.impact.warn_events}</strong>
              </div>
              <div className="text-center">
                <span className="text-xs text-[var(--muted)]">{t('timeline.failedActions')}</span>
                <strong className="text-lg font-medium text-[var(--text)] block">{groupTimeline.impact.failed_actions}</strong>
              </div>
              <div className="text-center">
                <span className="text-xs text-[var(--muted)]">{t('timeline.failedUpdates')}</span>
                <strong className="text-lg font-medium text-[var(--text)] block">{groupTimeline.impact.failed_updates}</strong>
              </div>
              <div className="text-center">
                <span className="text-xs text-[var(--muted)]">{t('timeline.unresponsive')}</span>
                <strong className="text-lg font-medium text-[var(--text)] block">{groupTimeline.impact.assets_stale}</strong>
              </div>
              <div className="text-center">
                <span className="text-xs text-[var(--muted)]">{t('timeline.offline')}</span>
                <strong className="text-lg font-medium text-[var(--text)] block">{groupTimeline.impact.assets_offline}</strong>
              </div>
            </div>
          ) : null}

          <ul className="divide-y divide-[var(--line)]">
            {(groupTimeline?.events ?? []).slice(0, timelineExpanded ? undefined : 18).map((event) => (
              <li key={event.id} className="flex items-center justify-between gap-3 py-2.5">
                <span className="text-sm font-medium text-[var(--text)]">
                  {formatTimestamp(event.timestamp)} / {event.title}
                  {event.summary ? ` / ${event.summary}` : ""}
                </span>
                <Badge status={event.severity === "error" ? "bad" : event.severity === "warn" ? "pending" : "ok"} size="sm" />
              </li>
            ))}
            {groupTimeline && groupTimeline.events.length === 0 ? <li className="py-2.5 text-xs text-[var(--muted)]">{t('timeline.noEvents')}</li> : null}
          </ul>
          {groupTimeline && groupTimeline.events.length > 18 ? (
            <Button
              variant="ghost"
              size="sm"
              className="mt-2"
              onClick={() => setTimelineExpanded((prev) => !prev)}
            >
              {timelineExpanded
                ? t('timeline.showLess')
                : `${t('timeline.showMore')} (${groupTimeline.events.length - 18})`}
            </Button>
          ) : null}
          {groupTimelineLoading ? (
            <div className="mt-3 space-y-1">
              <SkeletonRow />
              <SkeletonRow />
              <SkeletonRow />
            </div>
          ) : null}
          {groupTimelineError ? <p className="text-xs text-[var(--bad)]">{groupTimelineError}</p> : null}
        </Card>
      ) : null}

      {/* Maintenance tab */}
      {activeTab === "maintenance" ? (
        <Card className="mb-4">
          <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-3">{t('maintenance.heading')}</p>
          <form className="space-y-3 py-3 border-t border-[var(--line)]" onSubmit={createMaintenanceWindow}>
            <label className="flex flex-col gap-1">
              <span className="text-xs font-medium text-[var(--muted)]">{t('maintenance.group')}</span>
              <Select
                value={selectedTimelineGroup}
                onChange={(event) => setSelectedTimelineGroup(event.target.value)}
              >
                {groupRows.map((group) => (
                  <option key={group.id} value={group.id}>
                    {group.name}
                  </option>
                ))}
              </Select>
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-xs font-medium text-[var(--muted)]">{t('maintenance.name')}</span>
              <Input value={maintenanceName} onChange={(event) => setMaintenanceName(event.target.value)} />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-xs font-medium text-[var(--muted)]">{t('maintenance.start')}</span>
              <Input
                type="datetime-local"
                value={maintenanceStart}
                onChange={(event) => setMaintenanceStart(event.target.value)}
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-xs font-medium text-[var(--muted)]">{t('maintenance.end')}</span>
              <Input type="datetime-local" value={maintenanceEnd} onChange={(event) => setMaintenanceEnd(event.target.value)} />
            </label>
            <label className="flex items-center gap-2 text-sm text-[var(--text)]">
              <input
                type="checkbox"
                checked={maintenanceSuppressAlerts}
                onChange={(event) => setMaintenanceSuppressAlerts(event.target.checked)}
              />
              {t('maintenance.muteAlerts')}
            </label>
            <label className="flex items-center gap-2 text-sm text-[var(--text)]">
              <input
                type="checkbox"
                checked={maintenanceBlockActions}
                onChange={(event) => setMaintenanceBlockActions(event.target.checked)}
              />
              {t('maintenance.pauseActions')}
            </label>
            <label className="flex items-center gap-2 text-sm text-[var(--text)]">
              <input
                type="checkbox"
                checked={maintenanceBlockUpdates}
                onChange={(event) => setMaintenanceBlockUpdates(event.target.checked)}
              />
              {t('maintenance.pauseUpdates')}
            </label>
            <Button variant="primary" disabled={maintenanceSaving}>{maintenanceSaving ? t('maintenance.saving') : t('maintenance.submit')}</Button>
          </form>

          <ul className="divide-y divide-[var(--line)]">
            {maintenanceWindows.map((window) => {
              const start = new Date(window.start_at);
              const end = new Date(window.end_at);
              const now = new Date();
              const active = start <= now && end >= now;
              return (
                <li key={window.id} className="flex items-center justify-between gap-3 py-2.5">
                  <span className="text-sm font-medium text-[var(--text)]">
                    {window.name} / {start.toLocaleString()} - {end.toLocaleString()}
                    {window.block_actions ? " / pause actions" : ""}
                    {window.block_updates ? " / pause updates" : ""}
                  </span>
                  <div className="flex items-center gap-2">
                    <Badge status={active ? "pending" : "ok"} size="sm" />
                    <Button variant="ghost" size="sm" onClick={() => void deleteMaintenanceWindow(window.id)}>
                      Delete
                    </Button>
                  </div>
                </li>
              );
            })}
            {maintenanceWindows.length === 0 ? <li className="py-2.5 text-xs text-[var(--muted)]">{t('maintenance.empty')}</li> : null}
          </ul>
          {maintenanceLoading ? (
            <div className="mt-3 space-y-1">
              <SkeletonRow />
              <SkeletonRow />
            </div>
          ) : null}
          {maintenanceMessage ? <p className="text-xs text-[var(--muted)]">{maintenanceMessage}</p> : null}
        </Card>
      ) : null}

      {/* Map tab */}
      {activeTab === "map" ? (
        <Card className="mb-4">
          <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-3">{t('map.heading')}</p>
          <div className="relative h-64 bg-[var(--surface)] rounded-lg overflow-hidden mb-4">
            <WorldMapSVG className="absolute inset-0 w-full h-full" />
            {groupReliabilityRows
              .filter((entry) => typeof entry.group.latitude === "number" && typeof entry.group.longitude === "number")
              .map((entry) => {
                const lat = entry.group.latitude as number;
                const lon = entry.group.longitude as number;
                const left = ((lon + 180) / 360) * 100;
                const top = ((90 - lat) / 180) * 100;
                const markerClass = entry.score >= 90
                  ? "absolute text-[10px] px-1.5 py-0.5 rounded-lg border border-[var(--ok)]/30 bg-[var(--ok-glow)] text-[var(--ok)] -translate-x-1/2 -translate-y-1/2 transition-colors hover:bg-[var(--ok-glow)]"
                  : entry.score >= 75
                  ? "absolute text-[10px] px-1.5 py-0.5 rounded-lg border border-[var(--warn)]/30 bg-[var(--warn-glow)] text-[var(--warn)] -translate-x-1/2 -translate-y-1/2 transition-colors hover:bg-[var(--warn-glow)]"
                  : "absolute text-[10px] px-1.5 py-0.5 rounded-lg border border-[var(--bad)]/30 bg-[var(--bad-glow)] text-[var(--bad)] -translate-x-1/2 -translate-y-1/2 transition-colors hover:bg-[var(--bad-glow)]";
                return (
                  <button
                    key={entry.group.id}
                    className={markerClass}
                    style={{ left: `${left}%`, top: `${top}%` }}
                    title={`${entry.group.name} / score ${entry.score}`}
                    onClick={() => { setSelectedTimelineGroup(entry.group.id); setActiveTab("timeline"); }}
                  >
                    {entry.group.slug || entry.group.name.slice(0, 3).toUpperCase()}
                  </button>
                );
              })}
          </div>
          <ul className="divide-y divide-[var(--line)]">
            {groupReliabilityRows.map((entry) => (
              <li key={`map-${entry.group.id}`} className="flex items-center justify-between gap-3 py-2.5">
                <span className="text-sm font-medium text-[var(--text)]">
                  {entry.group.name}
                  {typeof entry.group.latitude === "number" && typeof entry.group.longitude === "number"
                    ? ` / ${entry.group.latitude.toFixed(3)}, ${entry.group.longitude.toFixed(3)}`
                    : ` / ${t('map.noCoordinates')}`}
                </span>
                <Badge status={entry.score >= 90 ? "ok" : entry.score >= 75 ? "pending" : "bad"} size="sm" />
              </li>
            ))}
            {groupReliabilityRows.length === 0 ? <li className="py-2.5 text-xs text-[var(--muted)]">{t('map.noLocationData')}</li> : null}
          </ul>
        </Card>
      ) : null}
    </>
  );
}

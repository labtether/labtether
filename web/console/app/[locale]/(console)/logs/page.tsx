"use client";

import { Link } from "../../../../i18n/navigation";
import { useTranslations } from "next-intl";
import { FileText } from "lucide-react";
import { PageHeader } from "../../../components/PageHeader";
import { Card } from "../../../components/ui/Card";
import { Badge } from "../../../components/ui/Badge";
import { EmptyState } from "../../../components/ui/EmptyState";
import { Input } from "../../../components/ui/Input";
import { Select } from "../../../components/ui/Input";
import { SegmentedTabs } from "../../../components/ui/SegmentedTabs";
import { SkeletonRow } from "../../../components/ui/Skeleton";
import { useLogs } from "../../../hooks/useLogs";
import { formatTimestamp } from "../../../console/formatters";
import { logLevels } from "../../../console/models";
import type { LogLevel } from "../../../console/models";
import { friendlySourceLabel } from "../../../console/taxonomy";

export default function LogsPage() {
  const t = useTranslations('logs');
  const {
    logSources,
    recentLogs,
    logWindow,
    setLogWindow,
    telemetryWindows,
    selectedLogSource,
    setSelectedLogSource,
    logLevel,
    setLogLevel,
    includeHeartbeats,
    setIncludeHeartbeats,
    logQuery,
    setLogQuery,
    logEvents,
    logsLoading,
    logsError
  } = useLogs();

  return (
    <>
      <PageHeader title={t('title')} subtitle={t('subtitle')} />
      <Card>
        <div className="flex flex-wrap items-end gap-4 mb-4">
          <div className="flex flex-col gap-1">
            <span className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">{t('filters.source')}</span>
            <Select
              value={selectedLogSource}
              onChange={(event) => setSelectedLogSource(event.target.value)}
            >
              <option value="all">{t('filters.allSources')}</option>
              {logSources.map((source) => (
                <option key={source.source} value={source.source}>
                  {friendlySourceLabel(source.source)} ({source.count})
                </option>
              ))}
            </Select>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">{t('filters.level')}</span>
            <Select value={logLevel} onChange={(event) => setLogLevel(event.target.value as LogLevel)}>
              {logLevels.map((level) => (
                <option key={level} value={level}>
                  {level.toUpperCase()}
                </option>
              ))}
            </Select>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">{t('filters.events')}</span>
            <SegmentedTabs
              value={includeHeartbeats ? "all" : "operational"}
              options={[
                { id: "operational", label: t('filters.operational') },
                { id: "all", label: t('filters.all') }
              ]}
              onChange={(value) => setIncludeHeartbeats(value === "all")}
            />
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">{t('filters.range')}</span>
            <SegmentedTabs
              value={logWindow}
              options={telemetryWindows.map((windowValue) => ({ id: windowValue, label: windowValue }))}
              onChange={setLogWindow}
            />
          </div>
          <div className="flex flex-col gap-1 flex-1">
            <span className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">{t('filters.search')}</span>
            <Input
              value={logQuery}
              onChange={(event) => setLogQuery(event.target.value)}
              placeholder={t('search.placeholder')}
            />
          </div>
        </div>

        <ul className="divide-y divide-[var(--line)]">
          {(logEvents.length > 0 ? logEvents : recentLogs).slice(0, 80).map((event) => (
            <li key={event.id} className="flex items-center gap-3 py-2">
              <span className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">{formatTimestamp(event.timestamp)}</span>
              <span className="text-sm font-medium text-[var(--text)]">{event.source}</span>
              <Badge
                status={event.level === "error" ? "bad" : event.level === "warn" ? "pending" : "ok"}
                size="sm"
                dot
              />
              <span className="text-xs text-[var(--text)] truncate flex-1">{event.message}</span>
            </li>
          ))}
        </ul>
        {logsLoading ? (
          <div className="mt-3 space-y-1">
            <SkeletonRow />
            <SkeletonRow />
            <SkeletonRow />
          </div>
        ) : null}
        {logsError ? <p className="text-xs text-[var(--bad)]">{logsError}</p> : null}
        {logEvents.length === 0 && recentLogs.length > 0 && !logsLoading && !logsError ? (
          <p className="text-xs text-[var(--muted)] mt-3">{t('noMatch')}</p>
        ) : null}
        {logEvents.length === 0 && recentLogs.length === 0 && !logsLoading ? (
          <EmptyState
            icon={FileText}
            title={t('empty.title')}
            description={t('empty.description')}
            action={<Link href="/nodes" className="text-xs text-[var(--accent)] hover:underline">{t('empty.viewDevices')}</Link>}
          />
        ) : null}
      </Card>
    </>
  );
}

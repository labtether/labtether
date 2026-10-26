import type { ReactNode } from "react";
import { useTranslations } from "next-intl";
import { Bell, Activity, FileText, RefreshCw, Zap } from "lucide-react";
import type { IncidentEvent } from "../../../console/models";
import { Button } from "../../../components/ui/Button";

function timelineKindIcon(kind: string): ReactNode {
  switch (kind) {
    case "alert_fired":
    case "alert_resolved":
      return <Bell size={14} />;
    case "metric_anomaly":
    case "heartbeat_change":
      return <Activity size={14} />;
    case "log_burst":
    case "audit":
      return <FileText size={14} />;
    case "config_change":
      return <RefreshCw size={14} />;
    default:
      return <Zap size={14} />;
  }
}

export type IncidentTimelineProps = {
  events: IncidentEvent[];
  loading: boolean;
  error: boolean;
  hasMore: boolean;
  onLoadOlder: () => void;
  onRetry: () => void;
  onRefresh: () => void;
};

export function IncidentTimeline({ events, loading, error, hasMore, onLoadOlder, onRetry, onRefresh }: IncidentTimelineProps) {
  const t = useTranslations("incidents.timeline");
  return (
    <section aria-label={t("title")} aria-busy={loading}>
      <div className="flex items-center justify-between gap-2 mb-2">
        <h3 className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">{t("title")}</h3>
        <Button size="sm" variant="ghost" disabled={loading} onClick={onRefresh}>{t("refresh")}</Button>
      </div>
      {events.length === 0 && !loading && !error ? <p className="text-xs text-[var(--muted)]">{t("empty")}</p> : null}
      {events.length > 0 ? (
        <ul className="divide-y divide-[var(--line)]">
          {events.map((entry) => (
            <li key={entry.id} className="flex items-start gap-3 py-2.5">
              <span aria-hidden="true" className="text-[var(--muted)] mt-0.5 shrink-0">{timelineKindIcon(entry.event_type)}</span>
              <div className="flex-1 min-w-0">
                <span className="text-sm text-[var(--text)] block">{entry.summary}</span>
              </div>
              <time dateTime={entry.occurred_at} className="text-xs text-[var(--muted)]">{new Date(entry.occurred_at).toLocaleString()}</time>
            </li>
          ))}
        </ul>
      ) : null}
      {loading ? <p role="status" className="text-xs text-[var(--muted)]">{t("loading")}</p> : null}
      {error ? <div role="alert" className="mt-2">
        <p className="text-xs text-[var(--bad)]">{t("error")}</p>
        <Button size="sm" disabled={loading} onClick={onRetry}>{t("retry")}</Button>
      </div> : null}
      {hasMore && !error ? <Button size="sm" disabled={loading} onClick={onLoadOlder}>{t("loadOlder")}</Button> : null}
    </section>
  );
}

import { useLocale, useTranslations } from "next-intl";
import type { IncidentEvent } from "../../../console/models";
import { Button } from "../../../components/ui/Button";
import { chronological, eventTone, fullWhen, shortWhen } from "./issuesModel";

export type IncidentTimelineProps = {
  events: IncidentEvent[];
  loading: boolean;
  error: boolean;
  hasMore: boolean;
  onLoadOlder: () => void;
  onRetry: () => void;
  onRefresh: () => void;
};

/**
 * What happened, oldest first: alerts firing (red dot) and recovering (green), things someone ran (solid), the rest
 * as rings. The hub pages it newest first, so older history loads above.
 */
export function IncidentTimeline({ events, loading, error, hasMore, onLoadOlder, onRetry, onRefresh }: IncidentTimelineProps) {
  const t = useTranslations("incidents.timeline");
  const locale = useLocale();
  return (
    <section className="iu-sec" aria-labelledby="iu-tl-h" aria-busy={loading}>
      <div className="iu-sh">
        <h2 id="iu-tl-h">{t("title")}</h2>
        <button type="button" className="iu-sh-link" disabled={loading} onClick={onRefresh}>{t("refresh")}</button>
      </div>
      {hasMore && !error ? (
        <Button size="sm" variant="ghost" className="iu-tl-more" disabled={loading} onClick={onLoadOlder}>{t("loadOlder")}</Button>
      ) : null}
      {events.length > 0 ? (
        <ol className="iu-tl">
          {chronological(events).map((entry) => (
            <li key={entry.id} className={eventTone(entry) || undefined}>
              <time dateTime={entry.occurred_at} title={fullWhen(entry.occurred_at, locale)}>{shortWhen(entry.occurred_at, locale)}</time>
              <span>{entry.summary}</span>
            </li>
          ))}
        </ol>
      ) : null}
      {events.length === 0 && !loading && !error ? <p className="iu-quiet">{t("empty")}</p> : null}
      {loading ? <p role="status" className="iu-quiet">{t("loading")}</p> : null}
      {error ? (
        <div className="iu-inline-err">
          <p role="alert">{t("error")}</p>
          <Button size="sm" disabled={loading} onClick={onRetry}>{t("retry")}</Button>
        </div>
      ) : null}
    </section>
  );
}

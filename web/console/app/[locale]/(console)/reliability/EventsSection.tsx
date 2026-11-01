"use client";

import { forwardRef, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Select } from "../../../components/ui/Input";
import { groupTimelineWindows, type Group, type GroupTimelineResponse, type GroupTimelineWindow } from "../../../console/models";
import { groupPath } from "../groups/groupListModel";
import { Section, Status } from "./HealthParts";
import { when } from "./healthTime";

type EventsSectionProps = {
  groups: Group[];
  groupId: string;
  onGroup: (id: string) => void;
  range: GroupTimelineWindow;
  onRange: (range: GroupTimelineWindow) => void;
  timeline: GroupTimelineResponse | null;
  loading: boolean;
  failed: boolean;
};

const SHOWN = 18;
const SEVERITY = { error: { tone: "bad", word: "error" }, warn: { tone: "warn", word: "warning" }, info: { tone: "none", word: "info" } } as const;

/** Events: one group's timeline over the last hour, 6 hours or day, with what went wrong counted in one line. */
export const EventsSection = forwardRef<HTMLElement, EventsSectionProps>(function EventsSection(
  { groups, groupId, onGroup, range, onRange, timeline, loading, failed },
  ref,
) {
  const t = useTranslations("reliability.events");
  const locale = useLocale();
  const [all, setAll] = useState(false);
  const now = Date.now();
  const events = timeline?.events ?? [];
  const impact = timeline?.impact;
  const counts = impact
    ? [
        impact.error_events > 0 ? t("impact.errors", { count: impact.error_events }) : "",
        impact.warn_events > 0 ? t("impact.warnings", { count: impact.warn_events }) : "",
        impact.failed_actions > 0 ? t("impact.failedActions", { count: impact.failed_actions }) : "",
        impact.failed_updates > 0 ? t("impact.failedUpdates", { count: impact.failed_updates }) : "",
        impact.assets_stale > 0 ? t("impact.stale", { count: impact.assets_stale }) : "",
        impact.assets_offline > 0 ? t("impact.offline", { count: impact.assets_offline }) : "",
      ].filter(Boolean)
    : [];

  const controls = (
    <>
      <Select value={groupId} onChange={(e) => onGroup(e.target.value)} aria-label={t("group")} className="pp-sel">
        {groups.map((g) => <option key={g.id} value={g.id}>{groupPath(groups, g.id)}</option>)}
      </Select>
      <span className="pp-seg" role="group" aria-label={t("range")}>
        {groupTimelineWindows.map((w) => (
          <button key={w} type="button" aria-pressed={w === range} onClick={() => onRange(w)}>{t(`ranges.${w}`)}</button>
        ))}
      </span>
    </>
  );

  return (
    <Section ref={ref} id="pp-events" title={t("title")} right={groups.length > 0 ? controls : null}>
      {impact ? <p className="pp-line">{counts.length ? counts.join(" · ") : t("quiet")}</p> : null}
      {failed ? <p className="pp-err pp-load" role="alert">{t("failed")}</p> : null}
      <div className="pp-list" role="table" aria-labelledby="pp-events">
        {(all ? events : events.slice(0, SHOWN)).map((event) => {
          const sev = SEVERITY[event.severity] ?? SEVERITY.info;
          return (
            <div key={event.id} className="pp-row pp-ev" role="row">
              <time className="pp-c pp-num" dateTime={event.timestamp} role="cell">{when(event.timestamp, now, locale)}</time>
              <span className="pp-in" role="cell">
                <span className="pp-nm">{event.title}</span>
                {event.summary ? <span className="pp-grey">{event.summary}</span> : null}
              </span>
              <span role="cell"><Status tone={sev.tone}>{t(`severity.${sev.word}`)}</Status></span>
            </div>
          );
        })}
        {!loading && timeline && events.length === 0 ? <div role="row"><p role="cell" className="pp-empty">{t("empty")}</p></div> : null}
        {loading && events.length === 0 ? <div role="row"><p role="cell" className="pp-empty">{t("loading")}</p></div> : null}
        {!groupId ? <div role="row"><p role="cell" className="pp-empty">{t("noGroups")}</p></div> : null}
        {events.length > SHOWN ? (
          <div className="pp-foot" role="row">
            <span role="cell">
              <button type="button" className="pp-link" onClick={() => setAll((v) => !v)}>
                {all ? t("showLess") : t("showAll", { count: events.length })}
              </button>
            </span>
          </div>
        ) : null}
      </div>
    </Section>
  );
});

"use client";

import { useLocale, useTranslations } from "next-intl";
import type { DeadLetterAnalytics, DeadLetterEvent } from "../../../console/models";
import { Section } from "./HealthParts";
import { clock, when } from "./healthTime";

type FailedJobsSectionProps = {
  deadLetters: DeadLetterEvent[];
  analytics: DeadLetterAnalytics;
};

/**
 * Failed jobs: background work the hub gave up on, newest first, the errors that come back most, and the count per
 * hour. Shown only when there is something to show; the Hub list carries the all-clear.
 */
export function FailedJobsSection({ deadLetters, analytics }: FailedJobsSectionProps) {
  const t = useTranslations("reliability.jobs");
  const locale = useLocale();
  if (deadLetters.length === 0 && analytics.total === 0 && analytics.top_error_classes.length === 0) return null;
  const now = Date.now();
  const rate = new Intl.NumberFormat(locale, { maximumFractionDigits: 2 });
  const recent = [...deadLetters].sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at)).slice(0, 8);
  const trend = analytics.trend.slice(-6);

  return (
    <Section
      id="pp-jobs"
      title={t("title")}
      right={<span className="pp-meta">{t("rate", { hour: rate.format(analytics.rate_per_hour), day: rate.format(analytics.rate_per_day) })}</span>}
    >
      <div className="pp-list" role="table" aria-labelledby="pp-jobs">
        {recent.map((event) => (
          <div key={event.id} className="pp-row pp-fj" role="row">
            <span className="pp-in" role="cell">
              <span className="pp-nm">{event.component || t("worker")}</span>
              <span className="pp-grey">{event.error || event.subject}</span>
            </span>
            <span className="pp-c pp-r pp-num" role="cell">{when(event.created_at, now, locale)}</span>
          </div>
        ))}
        {recent.length === 0 ? <div role="row"><p role="cell" className="pp-empty">{t("none")}</p></div> : null}
      </div>
      {analytics.top_error_classes.length > 0 ? (
        <>
          <h3 className="pp-sub">{t("common")}</h3>
          <div className="pp-list" role="table" aria-label={t("common")}>
            {analytics.top_error_classes.slice(0, 4).map((entry) => (
              <div key={entry.key} className="pp-row pp-fj" role="row">
                <span className="pp-nm pp-wrap" role="cell">{entry.key}</span>
                <span className="pp-c pp-r pp-num" role="cell">{t("times", { count: entry.count })}</span>
              </div>
            ))}
          </div>
        </>
      ) : null}
      {trend.length > 0 ? (
        <p className="pp-line pp-trend">
          {t("byHour")}{" "}
          {trend.map((point) => t("hourCount", { count: point.count, time: clock(point.start, locale) })).join(" · ")}
        </p>
      ) : null}
    </Section>
  );
}

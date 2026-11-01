"use client";

import { useCallback, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "../../../../i18n/navigation";
import { Button } from "../../../components/ui/Button";
import { useAuth } from "../../../contexts/AuthContext";
import { patchIncident, useIncidentDetail } from "../../../hooks/useIncidents";
import { hasWriteRole } from "../../../lib/roles";
import { IncidentTimeline } from "./IncidentTimeline";
import { IssueDevice } from "./IssueDevice";
import { IssueMenus } from "./IssueMenus";
import { IssueNotes } from "./IssueNotes";
import { IssueWriteup } from "./IssueWriteup";
import { durationParts, fullWhen, hasWriteup, isActive, openedAt, severityTone, shortWhen, type Issue, type IssueStatus } from "./issuesModel";

/** "1 hour 29 minutes", "2 days 3 hours": the two largest parts. */
function useDuration() {
  const t = useTranslations("incidents.duration");
  return (from: string, to: string) => {
    const p = durationParts(from, to);
    if (!p) return "";
    const parts = [p.days && t("days", { count: p.days }), p.hours && t("hours", { count: p.hours }), p.minutes && t("minutes", { count: p.minutes })];
    return parts.filter(Boolean).slice(0, 2).join(" ");
  };
}

/**
 * One issue: its title and severity, state and owner as header menus, the write-up once resolved, the device it is
 * about, the timeline and the notes. Changes save straight to the hub; a refused change says so above the page.
 */
export function IssueView({ id }: { id: string }) {
  const t = useTranslations("incidents");
  const locale = useLocale();
  const duration = useDuration();
  const { user } = useAuth();
  const canWrite = hasWriteRole(user?.role);
  const { incident, loading, error, retryDetail, replaceIncident, timeline } = useIncidentDetail(id);
  const issue = incident as Issue | null;
  const [saveError, setSaveError] = useState(false);

  const save = useCallback(async (req: Record<string, unknown>) => {
    setSaveError(false);
    try {
      replaceIncident(await patchIncident(id, req));
      return true;
    } catch {
      setSaveError(true);
      return false;
    }
  }, [id, replaceIncident]);

  const crumb = (
    <nav className="iu-crumb" aria-label={t("detail.crumbs")}>
      <Link href="/incidents">{t("title")}</Link>
      {issue ? <><span aria-hidden="true">/</span><span className="iu-crumb-here" aria-current="page">{issue.title}</span></> : null}
    </nav>
  );

  if (loading) {
    return <>{crumb}<div className="iu-skel-head" aria-hidden="true" /><div className="iu-list" aria-busy="true"><div className="iu-skel" /><div className="iu-skel" /><div className="iu-skel" /></div></>;
  }
  if (error || !issue) {
    return (
      <>
        {crumb}
        <div className="iu-sheet iu-gone">
          <p role="alert">{t("detail.error")}</p>
          <Button onClick={retryDetail}>{t("detail.retry")}</Button>
        </div>
      </>
    );
  }

  const opened = openedAt(issue);
  const sevTone = severityTone(issue.severity);
  const resolvedAt = !isActive(issue) && issue.resolved_at ? issue.resolved_at : "";
  const when = shortWhen(opened, locale);
  const meta = [
    issue.source === "alert_auto" ? t("detail.openedFromAlert", { time: when }) : t("detail.opened", { time: when }),
    resolvedAt ? t("detail.resolvedAfter", { duration: duration(opened, resolvedAt) }) : "",
  ].filter(Boolean).join(" · ");
  const showWriteup = !isActive(issue) || hasWriteup(issue);

  return (
    <>
      {crumb}
      <header className="lt-ph iu-ph">
        <div className="lt-ph-row">
          <div className="iu-ti">
            <h1>{issue.title}</h1>
            <span className={`iu-sev${sevTone ? ` ${sevTone}` : ""}`}>
              {sevTone ? <i className={`iu-dot ${sevTone}`} aria-hidden="true" /> : null}
              {t(`severity.${issue.severity}`)}
            </span>
          </div>
          <IssueMenus
            issue={issue}
            me={user?.username}
            disabled={!canWrite}
            onState={(status: IssueStatus) => void save({ status })}
            onOwner={(owner) => void save({ assignee: owner })}
          />
        </div>
        <p className="lt-ph-sub iu-meta" title={fullWhen(opened, locale)}>{meta}</p>
      </header>
      {saveError ? <p role="alert" className="iu-alert">{t("detail.saveError")}</p> : null}
      <div className="iu-page">
        {showWriteup ? <IssueWriteup issue={issue} canWrite={canWrite} primary={!isActive(issue)} onSave={save} /> : null}
        {issue.primary_asset_id ? <IssueDevice id={issue.primary_asset_id} /> : null}
        <IncidentTimeline {...timeline} />
        <IssueNotes issue={issue} canWrite={canWrite} onSave={save} />
      </div>
    </>
  );
}

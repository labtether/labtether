"use client";

import { Fragment, useState } from "react";
import { ChevronRight } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import type { UpdateRun } from "../../../console/models";
import { fullWhen, shortWhen } from "../incidents/issuesModel";
import { isLive, newestFirst, runTone, tally } from "./updatesModel";

/** "3 updated · 1 failed", or the run's state while it has no results. */
function useResultText() {
  const t = useTranslations("updates");
  return (run: UpdateRun) => {
    if (isLive(run) || !run.results?.length) return t.has(`status.${run.status}`) ? t(`status.${run.status}`) : run.status;
    const c = tally(run.results);
    return [c.succeeded && t("result.succeeded", { count: c.succeeded }), c.failed && t("result.failed", { count: c.failed }), c.other && t("result.other", { count: c.other })]
      .filter(Boolean).join(" · ");
  };
}

/** Every run, newest first: when, the plan, the result and who ran it. A row opens to what each device got. */
export function RunHistory({ runs, deviceNames }: { runs: UpdateRun[]; deviceNames: Map<string, string> }) {
  const t = useTranslations("updates");
  const locale = useLocale();
  const resultText = useResultText();
  const [open, setOpen] = useState<Set<string>>(() => new Set());
  const flip = (id: string) => setOpen((current) => {
    const next = new Set(current);
    if (next.has(id)) next.delete(id); else next.add(id);
    return next;
  });

  if (!runs.length) return <div className="iu-list"><div className="iu-empty"><b>{t("history.empty")}</b></div></div>;

  return (
    <div className="iu-list" role="table" aria-label={t("history.label")}>
      <div role="row" className="iu-row iu-hr iu-cols">
        <span role="columnheader" />
        <span role="columnheader">{t("history.columns.when")}</span>
        <span role="columnheader">{t("history.columns.plan")}</span>
        <span role="columnheader">{t("history.columns.result")}</span>
        <span role="columnheader">{t("history.columns.by")}</span>
      </div>
      {newestFirst(runs).map((run) => {
        const isOpen = open.has(run.id);
        const tone = runTone(run);
        return (
          <Fragment key={run.id}>
            <div role="row" className="iu-row iu-hr" onClick={(e) => { if (!(e.target as HTMLElement).closest("button")) flip(run.id); }}>
              <span role="cell">
                <button
                  type="button"
                  className={`iu-x${isOpen ? " open" : ""}`}
                  aria-expanded={isOpen}
                  aria-label={t(isOpen ? "history.hide" : "history.show", { plan: run.plan_name })}
                  onClick={() => flip(run.id)}
                >
                  <ChevronRight aria-hidden="true" strokeWidth={2} />
                </button>
              </span>
              <time role="cell" className="iu-num" dateTime={run.created_at} title={fullWhen(run.created_at, locale)}>{shortWhen(run.created_at, locale)}</time>
              <div role="cell" className="iu-main">
                <span className="iu-nm plain">{run.plan_name}</span>
                {run.dry_run ? <span className="iu-grey">{t("history.preview")}</span> : null}
                <span className="iu-grey iu-hr-when" aria-hidden="true">{shortWhen(run.created_at, locale)}</span>
              </div>
              <span role="cell" className={`iu-res${tone === "bad" ? " bad" : ""}`}>{resultText(run)}</span>
              <span role="cell" className="iu-c3 iu-by">{run.actor_id}</span>
            </div>
            {isOpen ? (
              <div role="row" className="iu-hd-row"><div role="cell" className="iu-hd">
                {run.error ? <p className="iu-hd-err">{run.error}</p> : null}
                {run.summary ? <p className="iu-hd-sum">{run.summary}</p> : null}
                {(run.results ?? []).map((r, i) => (
                  <div key={`${r.target}-${r.scope}-${i}`} className="iu-hi">
                    <span className="iu-nm plain">{deviceNames.get(r.target) ?? r.target}</span>
                    <span className={r.status === "failed" ? "iu-res bad" : "iu-res"}>{t.has(`status.${r.status}`) ? t(`status.${r.status}`) : r.status}</span>
                    <span className="iu-c3 iu-hi-s">{r.summary}</span>
                  </div>
                ))}
                {!run.error && !run.summary && !run.results?.length ? <p className="iu-hd-sum">{t("history.noResults")}</p> : null}
              </div></div>
            ) : null}
          </Fragment>
        );
      })}
    </div>
  );
}

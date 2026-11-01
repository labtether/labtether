"use client";

import { ChevronRight } from "lucide-react";
import { useTranslations } from "next-intl";
import type { CSSProperties } from "react";
import type { ActionRun } from "../../../../console/models";
import { formatLogTime, fullTime } from "./logsViewParts";

type NodeActionsTabCardProps = {
  actionsLoading: boolean;
  actionRuns: ActionRun[];
  expandedActionId: string | null;
  onExpandedActionChange: (nextID: string | null) => void;
};

const COLS = { "--cols": "minmax(0, 1fr) 120px 96px" } as CSSProperties;
const KNOWN_STATUSES = new Set(["succeeded", "failed", "running", "queued", "pending", "canceled"]);

/** A run's status as a tone (a dot only when it is not fine) and a translation key. */
function runStatus(status: string): { tone: "" | "bad" | "busy"; key: string } {
  const s = status.toLowerCase();
  if (s === "succeeded" || s === "success" || s === "completed") return { tone: "", key: "succeeded" };
  if (s === "failed" || s === "error") return { tone: "bad", key: "failed" };
  if (s === "cancelled") return { tone: "", key: "canceled" };
  return { tone: s === "running" || s === "queued" || s === "pending" ? "busy" : "", key: KNOWN_STATUSES.has(s) ? s : "" };
}

export function NodeActionsTabCard({
  actionsLoading,
  actionRuns,
  expandedActionId,
  onExpandedActionChange,
}: NodeActionsTabCardProps) {
  const t = useTranslations("devices.logsTab.actions");

  if (actionsLoading) {
    return <div className="dv-list"><p className="dst-quiet">{t("loading")}</p></div>;
  }
  if (actionRuns.length === 0) {
    return (
      <div className="dv-list">
        <div className="dv-empty"><b>{t("empty")}</b></div>
      </div>
    );
  }

  return (
    <div className="dv-list dst-runs" style={COLS}>
      <div className="dv-cols">
        <span>{t("columns.action")}</span>
        <span>{t("columns.status")}</span>
        <span className="r">{t("columns.time")}</span>
      </div>
      {actionRuns.map((run) => {
        const open = expandedActionId === run.id;
        const hasOutput = Boolean(run.output || run.error);
        const status = runStatus(run.status);
        const name = `${run.type}${run.action_id ? ` / ${run.action_id}` : ""}`;
        const what = run.command ?? run.connector_id ?? "";
        const toggle = () => onExpandedActionChange(open ? null : run.id);
        return (
          <div key={run.id} className="dst-run">
            <div className={`dv-row two${open ? " sel" : ""}`}>
              <div className="dst-run-main">
                {hasOutput ? (
                  <button type="button" className="dst-run-nm" aria-expanded={open} onClick={toggle}>
                    <ChevronRight aria-hidden="true" className={open ? "open" : ""} />
                    <span className="dv-nm">{name}</span>
                    <span className="dst-sr">{t(open ? "hideOutput" : "showOutput")}</span>
                  </button>
                ) : (
                  <span className="dst-run-nm"><span className="dst-chev-gap" aria-hidden="true" /><span className="dv-nm">{name}</span></span>
                )}
                {what ? <code className="dv-grey mono dst-run-cmd">{what}</code> : null}
              </div>
              <span className={`dst-st ${status.tone}`}>
                {status.tone ? <span className={`dv-dot ${status.tone}`} aria-hidden="true" /> : null}
                {status.key ? t(`status.${status.key}`) : run.status}
              </span>
              <time className="dv-n" dateTime={run.created_at} title={run.created_at ? fullTime(run.created_at) : undefined}>
                {run.created_at ? formatLogTime(run.created_at) : "—"}
              </time>
            </div>
            {open && hasOutput ? (
              <pre className="dst-out">
                {run.error ? <span className="dst-out-err">{t("errorLine", { error: run.error })}{"\n"}</span> : null}
                {run.output ?? ""}
              </pre>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

"use client";

import { useLocale, useTranslations } from "next-intl";
import { Fragment, useState } from "react";
import type { SavedAction } from "../../../hooks/useSavedActions";
import { pastWhen } from "./_parts/autoTime";
import { SortHead, type SortState } from "./_parts/Editor";
import { RowMenu } from "./_parts/RowMenu";
import { actionDevices, sortedActions, type SessionRun } from "./actionsModel";
import { SavedActionPanel } from "./SavedActionPanel";

/** The state word of a run started here: "Running", "Failed · 14:02", "Succeeded · 14:02". Nothing before a run. */
function RunState({ run }: { run?: SessionRun }) {
  const t = useTranslations("actions.list");
  const locale = useLocale();
  if (!run) return <span className="au-last" />;
  if (run.state === "running") return <span className="au-last"><span className="au-word busy">{t("running")}</span></span>;
  const time = pastWhen(run.at, new Date(), locale);
  if (run.state === "refused" || run.failed > 0) {
    return <span className="au-last"><span className="au-word bad">{t("failed")}</span><span className="num"> · {time}</span></span>;
  }
  return <span className="au-last">{t("succeededAt", { time })}</span>;
}

/**
 * Saved command actions as one list: name, steps and devices, the state of a run started here, Run on hover, and ⋯
 * for the rest. Opening a row loads the action's current steps and shows them with the last run's results.
 */
export function SavedActionList({
  actions,
  loading,
  error,
  runs,
  openID,
  openError,
  deviceName,
  onOpen,
  onRun,
  onDuplicate,
  onDelete,
  onRetry,
}: {
  actions: SavedAction[];
  loading: boolean;
  error: string | null;
  runs: Record<string, SessionRun>;
  openID: string | null;
  openError: string | null;
  deviceName: (id: string) => string;
  onOpen: (action: SavedAction | null) => void;
  onRun: (action: SavedAction) => void;
  onDuplicate: (action: SavedAction) => void;
  onDelete: (action: SavedAction) => void;
  onRetry: () => void;
}) {
  const t = useTranslations("actions.list");
  const tc = useTranslations("actions.common");
  const [sort, setSort] = useState<SortState<"name">>({ key: "name", dir: "asc" });
  const rows = sortedActions(actions, sort.dir);

  const sub = (action: SavedAction) => {
    const devices = actionDevices(action).map(deviceName).join(", ");
    return action.steps.length > 1 ? `${t("steps", { count: action.steps.length })} · ${devices}` : devices;
  };
  const toggle = (action: SavedAction) => onOpen(openID === action.id ? null : action);

  return (
    <div className="au-list" role="region" aria-label={t("label")}>
      <div className="au-cols au-a">
        <SortHead label={t("action")} column="name" sort={sort} onSort={setSort} />
        <span />
        <span />
        <span />
      </div>
      {loading && actions.length === 0 ? <p className="au-empty" role="status">{tc("loading")}</p> : null}
      {error ? (
        <div className="au-empty">
          <p role="alert" className="au-err">{t("loadFailed")}</p>
          <button type="button" className="au-link" onClick={onRetry}>{tc("retry")}</button>
        </div>
      ) : null}
      {!loading && !error && actions.length === 0 ? <p className="au-empty">{t("empty")}</p> : null}
      {rows.map((action) => {
        const open = openID === action.id;
        const run = runs[action.id];
        const busy = run?.state === "running";
        return (
          <Fragment key={action.id}>
            <div
              className={`au-row au-a${open ? " open" : ""}`}
              onClick={(event) => { if (!(event.target as HTMLElement).closest("button, a")) toggle(action); }}
            >
              <span className="au-ln">
                <button
                  type="button"
                  className="au-nm au-tg"
                  aria-expanded={open}
                  aria-controls={open ? `au-panel-${action.id}` : undefined}
                  onClick={() => toggle(action)}
                >{action.name}</button>
                <span className="au-sub">{sub(action)}</span>
              </span>
              <RunState run={run} />
              <button
                type="button"
                className="au-q"
                disabled={busy}
                aria-label={t("runLabel", { name: action.name })}
                onClick={() => onRun(action)}
              >
                {t("run")}
              </button>
              <RowMenu
                label={tc("rowMenu", { name: action.name })}
                items={[
                  { label: actionDevices(action).length > 1 ? t("runMany") : t("run"), onSelect: () => onRun(action), disabled: busy },
                  { label: open ? t("hideSteps") : t("showSteps"), onSelect: () => toggle(action) },
                  { label: t("duplicate"), onSelect: () => onDuplicate(action) },
                  { separator: true },
                  { label: tc("deleteMenu"), onSelect: () => onDelete(action), danger: true },
                ]}
              />
            </div>
            {open ? <SavedActionPanel action={action} run={run} deviceName={deviceName} loadError={openError} /> : null}
          </Fragment>
        );
      })}
    </div>
  );
}

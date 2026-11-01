"use client";

import { useLocale, useTranslations } from "next-intl";
import { useMemo, useState } from "react";
import { futureWhen, pastWhen } from "../actions/_parts/autoTime";
import { SortHead } from "../actions/_parts/Editor";
import { RowMenu, Switch } from "../actions/_parts/RowMenu";
import { browserZone, presetOf, splitZone } from "./cronModel";
import { lastRun, type Schedule, type ScheduleSort, sortSchedules } from "./scheduleModel";

/**
 * Schedules as one list: name over when it runs, the command and where; next run; the last run as a word and a time;
 * an On switch; ⋯ for Edit and Delete. A failed last run puts the hub's error in place of the grey line.
 */
export function ScheduleList({
  schedules,
  loading,
  failed,
  deviceName,
  groupName,
  onEdit,
  onToggle,
  onDelete,
  onRetry,
}: {
  schedules: Schedule[];
  loading: boolean;
  failed: boolean;
  deviceName: (id: string) => string;
  groupName: (id: string) => string;
  onEdit: (schedule: Schedule) => void;
  onToggle: (schedule: Schedule, on: boolean) => void;
  onDelete: (schedule: Schedule) => void;
  onRetry: () => void;
}) {
  const t = useTranslations("schedules.list");
  const te = useTranslations("schedules.editor");
  const tc = useTranslations("actions.common");
  const locale = useLocale();
  const zone = useMemo(() => browserZone(), []);
  const [sort, setSort] = useState<ScheduleSort>({ key: "next", dir: "asc" });
  const rows = sortSchedules(schedules, sort);
  const now = new Date();

  const whenWords = (expr: string) => {
    const preset = presetOf(expr, zone);
    return preset ? te(`presets.${preset}`) : t("cronLine", { cron: splitZone(expr).spec });
  };
  const where = (s: Schedule) => {
    const devices = s.targets.map(deviceName);
    const shown = devices.length > 2 ? `${devices.slice(0, 2).join(", ")} ${t("more", { count: devices.length - 2 })}` : devices.join(", ");
    return [shown, s.group_id ? t("group", { group: groupName(s.group_id) }) : ""].filter(Boolean).join(" · ");
  };

  return (
    <div className="au-list" role="region" aria-label={t("label")}>
      <div className="au-cols au-s">
        <SortHead label={t("schedule")} column="name" sort={sort} onSort={setSort} />
        <SortHead label={t("next")} column="next" sort={sort} onSort={setSort} />
        <SortHead label={t("last")} column="last" sort={sort} onSort={setSort} />
        <span>{tc("on")}</span>
        <span />
      </div>
      {loading && schedules.length === 0 ? <p className="au-empty" role="status">{tc("loading")}</p> : null}
      {failed ? (
        <div className="au-empty">
          <p role="alert" className="au-err">{t("loadFailed")}</p>
          <button type="button" className="au-link" onClick={onRetry}>{tc("retry")}</button>
        </div>
      ) : null}
      {!loading && !failed && schedules.length === 0 ? <p className="au-empty">{t("empty")}</p> : null}
      {rows.map((s) => {
        const last = lastRun(s);
        const lastTime = pastWhen(s.last_run_at, now, locale);
        const problem = (last.tone === "bad" || last.tone === "warn") && s.last_error ? s.last_error : "";
        const sub = problem || [whenWords(s.cron_expr), s.command, where(s) || t("noTargets")].join(" · ");
        return (
          <div key={s.id} className={`au-row au-s${s.enabled ? "" : " off"}`} onClick={(e) => { if (!(e.target as HTMLElement).closest("button, a")) onEdit(s); }}>
            <span className="au-ln">
              <button type="button" className="au-nm au-tg" onClick={() => onEdit(s)}>{s.name}</button>
              <span className={`au-sub${problem ? ` ${last.tone}` : ""}`} title={problem || undefined}>{sub}</span>
            </span>
            <span className="au-next num">{s.enabled ? futureWhen(s.next_run_at, now, locale) || "" : <span className="au-dim">{t("paused")}</span>}</span>
            <span className="au-last">
              {last.word === "never" ? <span className="au-dim">{t("never")}</span>
                : last.word === "ok" ? <span className="num">{lastTime}</span>
                  : (
                    <>
                      <span className={`au-word ${last.tone}`}>{t(`status.${last.word}`)}</span>
                      {lastTime && last.tone !== "busy" ? <span className="num"> · {lastTime}</span> : null}
                    </>
                  )}
            </span>
            <Switch on={s.enabled} label={t("onLabel", { name: s.name })} onChange={(on) => onToggle(s, on)} />
            <RowMenu
              label={tc("rowMenu", { name: s.name })}
              items={[
                { label: tc("edit"), onSelect: () => onEdit(s) },
                { label: s.enabled ? t("pause") : t("resume"), onSelect: () => onToggle(s, !s.enabled) },
                { separator: true },
                { label: tc("deleteMenu"), onSelect: () => onDelete(s), danger: true },
              ]}
            />
          </div>
        );
      })}
    </div>
  );
}

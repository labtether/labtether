"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import type { ProxmoxTask } from "../nodeDetailTypes";
import { proxmoxTaskState } from "../proxmoxFormatters";
import { DiPick, DiRowMenu, useDiConfirm } from "./DiControls";
import { DiCols, DiDots, DiEmpty, DiFoot, DiList, DiLog, DiMain, DiNote, DiRow, DiTime } from "./DiList";
import { spanText, whenText } from "./diFormat";
import { useProxmoxTaskLog } from "./useProxmoxTaskLog";

type Props = {
  tasks: ProxmoxTask[];
  proxmoxCollectorID: string;
  onRetry: () => void;
};

type StatusFilter = "all" | "running" | "ok" | "failed";
const STATUS_FILTERS: StatusFilter[] = ["all", "running", "ok", "failed"];
const PREVIEW = 30;

/** Recent Proxmox tasks: type, target and user, a status word and start time; a row opens its log. */
export function ProxmoxTasksTab({ tasks, proxmoxCollectorID, onRetry }: Props) {
  const t = useTranslations("devices-proxmox.pve.tasks");
  const kit = useTranslations("devices-proxmox.kit");
  const locale = useLocale();
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [typeFilter, setTypeFilter] = useState("");
  const [showAll, setShowAll] = useState(false);
  const [stopFailed, setStopFailed] = useState(false);
  const { openRef, logs, toggle, stop } = useProxmoxTaskLog(proxmoxCollectorID);
  const { ask, dialog } = useDiConfirm();

  const types = Array.from(new Set(tasks.map((task) => task.type ?? "").filter(Boolean))).sort();
  const filtered = tasks.filter((task) => {
    if (statusFilter !== "all" && proxmoxTaskState(task) !== statusFilter) return false;
    if (typeFilter && task.type !== typeFilter) return false;
    return true;
  });
  const shown = showAll ? filtered : filtered.slice(0, PREVIEW);
  const narrowed = statusFilter !== "all" || typeFilter !== "";

  return (
    <>
      <div className="di-tools">
        <DiPick
          label={t("statusLabel")}
          options={STATUS_FILTERS.map((id) => ({ id, label: t(`status.${id}`) }))}
          value={statusFilter}
          onChange={setStatusFilter}
        />
        {types.length > 0 ? (
          <DiPick
            label={t("typeLabel")}
            options={[{ id: "", label: t("allTypes") }, ...types.map((type) => ({ id: type, label: type }))]}
            value={typeFilter}
            onChange={setTypeFilter}
          />
        ) : null}
      </div>
      {stopFailed ? <DiNote tone="bad">{t("stopFailed")}</DiNote> : null}

      <DiList cols="minmax(0, 1fr) 120px 112px 28px" colsS="minmax(0, 1fr) auto 28px" label={t("label")}>
        <DiCols>
          <span>{t("task")}</span>
          <span className="di-s">{t("state")}</span>
          <span className="r">{t("started")}</span>
          <span />
        </DiCols>
        {shown.length === 0 ? <DiEmpty title={narrowed ? t("noMatch") : t("none")} /> : null}
        {shown.map((task, index) => {
          const state = proxmoxTaskState(task);
          const canLog = !!(task.upid && task.node);
          const ref = canLog ? `${task.node}/${task.upid}` : "";
          const open = canLog && openRef === ref;
          const log = logs[ref];
          const took = state !== "running" && task.endtime && task.starttime ? spanText(task.endtime - task.starttime) : "";
          const exit = state === "failed" && task.exitstatus && task.exitstatus.toLowerCase() !== "ok" ? task.exitstatus : "";
          const name = task.type || t("task");
          return (
            <DiRow
              key={`${task.upid ?? "task"}-${index}`}
              tone={state === "running" ? "busy" : state === "failed" ? "bad" : ""}
              onOpen={canLog ? () => void toggle(task.node!, task.upid!) : undefined}
              selected={open}
              after={open ? (
                <DiLog label={t("logOf", { name })}>
                  {log?.loading ? kit("loadingLog") : log?.failed ? kit("logFailed") : log?.text || kit("noLog")}
                </DiLog>
              ) : null}
            >
              <DiMain
                name={name}
                grey={<DiDots parts={[task.id || null, task.user || null, took]} />}
                problem={exit ? { tone: "bad", text: exit } : null}
              />
              <span className="di-s">
                <span className={`di-word${state === "failed" ? " bad" : ""}`}>{t(`status.${state}`)}</span>
              </span>
              <DiTime>{whenText(task.starttime, locale)}</DiTime>
              {canLog ? (
                <DiRowMenu
                  label={kit("actionsFor", { name })}
                  items={[
                    { label: open ? kit("hideLog") : kit("log"), onSelect: () => void toggle(task.node!, task.upid!) },
                    ...(state === "running"
                      ? [
                          { separator: true as const },
                          {
                            label: t("stop"),
                            danger: true,
                            onSelect: () => ask({
                              title: t("stopTitle", { name }),
                              message: t("stopBody"),
                              confirmLabel: t("stopConfirm"),
                              onConfirm: async () => {
                                const ok = await stop(task.node!, task.upid!);
                                setStopFailed(!ok);
                                onRetry();
                              },
                            }),
                          },
                        ]
                      : []),
                  ]}
                />
              ) : (
                <span />
              )}
            </DiRow>
          );
        })}
        {filtered.length > PREVIEW || narrowed ? (
          <DiFoot
            left={kit("shown", { shown: shown.length, total: narrowed ? tasks.length : filtered.length })}
            right={
              narrowed ? (
                <button type="button" onClick={() => { setStatusFilter("all"); setTypeFilter(""); }}>{kit("clearFilter")}</button>
              ) : (
                <button type="button" onClick={() => setShowAll((value) => !value)}>{showAll ? kit("showFewer") : kit("showAll")}</button>
              )
            }
          />
        ) : null}
      </DiList>
      {dialog}
    </>
  );
}

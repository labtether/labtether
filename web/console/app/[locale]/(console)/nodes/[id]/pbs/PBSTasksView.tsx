"use client";

// Tasks: the server's recent tasks, newest first, with a type menu and All / Errors / Running. A task opens under
// its row with its state and log; a running one can be stopped after a confirm.

import { useMemo, useState } from "react";

import { DiSeg } from "../proxmox/DiControls";
import { DiCols, DiDots, DiEmpty, DiFoot, DiList, DiRow, DiSection, DiTime, type DiTone } from "../proxmox/DiList";
import { spanText } from "../proxmox/diFormat";
import type { PBSAssetDetailsResponse, PBSTask } from "../pbsTabModel";
import { PBSSelect } from "./PBSSelect";
import { PBSTaskPanel } from "./PBSTaskPanel";
import { filterTasks, taskState, type TaskFilter } from "./pbsViewModel";
import { usePBSWords } from "./usePBSWords";

type Props = {
  details: PBSAssetDetailsResponse;
  onRefreshDetails: () => void;
};

const COLS = "minmax(0, 1fr) 72px 120px";
const COLS_S = "minmax(0, 1fr) 96px";
const LIMIT = 30;

export function PBSTasksView({ details, onRefreshDetails }: Props) {
  const { t, tk, when, kindLabel } = usePBSWords();
  const tasks = useMemo(() => details.tasks ?? [], [details]);
  const [type, setType] = useState("");
  const [filter, setFilter] = useState<TaskFilter>("all");
  const [open, setOpen] = useState<string | null>(null);
  const [all, setAll] = useState(false);

  const types = useMemo(() => Array.from(new Set(tasks.map((task) => task.worker_type || "unknown"))).sort(), [tasks]);
  const shown = filterTasks(tasks, type, filter);
  const visible = all ? shown : shown.slice(0, LIMIT);
  const collectorID = details.collector_id?.trim() ?? "";
  const nodeFallback = details.node?.trim() || "localhost";

  const toneOf = (task: PBSTask): DiTone => {
    const state = taskState(task);
    return state === "failed" ? "bad" : state === "running" ? "busy" : "";
  };

  return (
    <DiSection id="pbs-tasks" title={t("tasks.title")}>
      <div className="di-tools">
        <PBSSelect label={t("tasks.type")} value={type} onChange={setType}>
          <option value="">{t("tasks.allTypes")}</option>
          {types.map((name) => <option key={name} value={name}>{kindLabel(name)}</option>)}
        </PBSSelect>
        <DiSeg<TaskFilter>
          label={t("tasks.state")}
          value={filter}
          onChange={setFilter}
          options={[{ id: "all", label: t("tasks.all") }, { id: "errors", label: t("tasks.errors") }, { id: "running", label: t("tasks.running") }]}
        />
      </div>
      <DiList cols={COLS} colsS={COLS_S} hang label={t("tasks.title")}>
        <DiCols>
          <span>{t("cols.task")}</span>
          <span className="r di-s">{t("cols.took")}</span>
          <span className="r">{t("cols.started")}</span>
        </DiCols>
        {visible.length === 0 ? <DiEmpty title={tasks.length === 0 ? t("tasks.none") : t("tasks.noMatch")} /> : null}
        {visible.map((task) => {
          const expanded = open === task.upid;
          const state = taskState(task);
          const toggle = () => setOpen(expanded ? null : task.upid);
          const took = task.starttime && task.endtime ? spanText(task.endtime - task.starttime) : "";
          return (
            <DiRow
              key={task.upid}
              tone={toneOf(task)}
              onOpen={toggle}
              selected={expanded}
              after={expanded ? (
                <PBSTaskPanel
                  task={task}
                  collectorID={collectorID}
                  nodeFallback={nodeFallback}
                  onStopped={onRefreshDetails}
                />
              ) : null}
            >
              <span className="di-main">
                <button type="button" className="di-nm di-pbs-open" aria-expanded={expanded} onClick={toggle}>
                  {kindLabel(task.worker_type)}
                </button>
                {state === "failed" ? (
                  <span className="di-grey bad">{task.status}</span>
                ) : (
                  <span className="di-grey">
                    <DiDots parts={[state === "running" ? t("state.running") : null, task.worker_id, task.user]} />
                  </span>
                )}
              </span>
              <DiTime className="di-s">{took}</DiTime>
              <DiTime>{when(task.starttime)}</DiTime>
            </DiRow>
          );
        })}
        {shown.length > LIMIT || type !== "" || filter !== "all" ? (
          <DiFoot
            left={tk("shown", { shown: visible.length, total: tasks.length })}
            right={(
              <span className="di-acts">
                {shown.length > LIMIT ? (
                  <button type="button" onClick={() => setAll(!all)}>{all ? tk("showFewer") : tk("showAll")}</button>
                ) : null}
                {type !== "" || filter !== "all" ? (
                  <button type="button" onClick={() => { setType(""); setFilter("all"); }}>{tk("clearFilter")}</button>
                ) : null}
              </span>
            )}
          />
        ) : null}
      </DiList>
    </DiSection>
  );
}

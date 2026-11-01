"use client";

// The right-hand "last run" of a job row: the time when it ended well, "Failed 04:00" in red, "Running" with a
// live dot, or "Never".

import type { ReactNode } from "react";

import { DiStatus, DiTime } from "../proxmox/DiList";
import type { PBSTask } from "../pbsTabModel";
import { taskState } from "./pbsViewModel";
import { usePBSWords } from "./usePBSWords";

/** Grid of a job row: name and grey details, last run, ⋯; the last run hides on a phone. */
export const JOB_COLS = "minmax(0, 1fr) minmax(120px, auto) 28px";
export const JOB_COLS_S = "minmax(0, 1fr) 28px";

export function PBSLastRun({ task, extra, className = "" }: { task?: PBSTask; extra?: ReactNode; className?: string }) {
  const { t, tk, when } = usePBSWords();
  if (!task) return <DiTime className={`quiet ${className}`}>{tk("never")}</DiTime>;
  const state = taskState(task);
  if (state === "running") {
    return <span className={`di-pbs-last ${className}`}><DiStatus tone="busy" word={t("state.running")} /></span>;
  }
  if (state === "failed") {
    return <span className={`di-pbs-last ${className}`}><DiStatus tone="bad" word={t("state.failed")} time={when(task.starttime)} /></span>;
  }
  return (
    <DiTime className={className}>
      {when(task.starttime)}
      {extra ? <> · {extra}</> : null}
    </DiTime>
  );
}

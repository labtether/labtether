"use client";

// Sync: each sync job with where it pulls from, its options and schedule, and its last run from the task list.
// A job's ⋯ runs it now or deletes the job after a confirm.

import { DiRowMenu, useDiConfirm } from "../proxmox/DiControls";
import { DiCols, DiDots, DiEmpty, DiList, DiMain, DiNote, DiRow, DiSection } from "../proxmox/DiList";
import type { PBSTask } from "../pbsTabModel";
import { JOB_COLS, JOB_COLS_S, PBSLastRun } from "./PBSLastRun";
import { normalizeSyncJobs, type PBSSyncJob } from "./pbsJobsModel";
import { newestTask, taskState } from "./pbsViewModel";
import { pbsAction, pbsErrorDetail, usePBSActions, usePBSResource } from "./usePBSData";
import { usePBSWords } from "./usePBSWords";

const EMPTY: PBSSyncJob[] = [];

export function PBSSyncJobs({ assetId, reloadKey, tasks }: { assetId: string; reloadKey: number; tasks: PBSTask[] }) {
  const { t, tk, schedule } = usePBSWords();
  const base = `/api/pbs/assets/${encodeURIComponent(assetId)}/sync-jobs`;
  const jobs = usePBSResource(base, normalizeSyncJobs, EMPTY, reloadKey);
  const { busy, note, run } = usePBSActions();
  const { ask, dialog } = useDiConfirm();

  const nameOf = (job: PBSSyncJob) => t("jobs.syncName", { store: job.store });
  const runJob = (job: PBSSyncJob) => run(
    `run-${job.id}`,
    () => pbsAction(`${base}/${encodeURIComponent(job.id)}/run`, "POST"),
    t("jobs.syncStarted", { store: job.store }),
    t("jobs.runFailed"),
    jobs.refresh,
  );
  const deleteJob = (job: PBSSyncJob) => run(
    `delete-${job.id}`,
    () => pbsAction(`${base}/${encodeURIComponent(job.id)}`, "DELETE"),
    t("jobs.deleted", { name: nameOf(job) }),
    t("jobs.deleteFailed"),
    jobs.refresh,
  );

  return (
    <DiSection id="pbs-sync" title={t("jobs.sync")}>
      {jobs.error ? <DiNote tone="bad">{[t("jobs.loadFailed"), pbsErrorDetail(jobs.error)].filter(Boolean).join(" ")}</DiNote> : null}
      {note ? <DiNote tone={note.tone}>{note.text}</DiNote> : null}
      <DiList cols={JOB_COLS} colsS={JOB_COLS_S} hang label={t("jobs.sync")}>
        <DiCols>
          <span>{t("cols.job")}</span>
          <span className="r di-s">{t("cols.lastRun")}</span>
          <span />
        </DiCols>
        {jobs.data.length === 0 ? <DiEmpty title={jobs.loaded ? t("jobs.noSync") : tk("loading")} /> : null}
        {jobs.data.map((job) => {
          const name = nameOf(job);
          const last = newestTask(tasks, "sync", job.store);
          const from = [job.remote, job.remote_store].filter(Boolean).join(":");
          return (
            <DiRow key={job.id} tone={last && taskState(last) === "failed" ? "bad" : ""}>
              <DiMain
                name={name}
                grey={<DiDots parts={[
                  from ? t("jobs.syncFrom", { source: from }) : null,
                  schedule(job.schedule),
                  job.verified_only ? t("jobs.verifiedOnly") : null,
                  job.transfer_last != null ? t("jobs.transferLast", { count: job.transfer_last }) : null,
                  job.remove_vanished ? t("jobs.removeVanished") : null,
                  job.comment,
                ]} />}
              />
              <PBSLastRun task={last} className="di-s" />
              <DiRowMenu
                label={tk("actionsFor", { name })}
                items={[
                  { label: t("jobs.runNow"), disabled: busy !== null, onSelect: () => void runJob(job) },
                  { separator: true },
                  {
                    label: t("jobs.delete"),
                    danger: true,
                    disabled: busy !== null,
                    onSelect: () => ask({
                      title: t("jobs.ask.deleteTitle", { name }),
                      message: t("jobs.ask.deleteBody"),
                      confirmLabel: t("jobs.ask.delete"),
                      onConfirm: () => deleteJob(job),
                    }),
                  },
                ]}
              />
            </DiRow>
          );
        })}
      </DiList>
      {dialog}
    </DiSection>
  );
}

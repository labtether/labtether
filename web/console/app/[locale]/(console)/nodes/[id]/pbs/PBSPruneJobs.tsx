"use client";

// Prune: each prune job with what it keeps and when it runs, and its last run from the task list. A job's ⋯
// simulates it, runs it now (after a confirm, since it deletes snapshots) or deletes the job (after a confirm).

import { DiRowMenu, useDiConfirm } from "../proxmox/DiControls";
import { DiCols, DiDots, DiEmpty, DiList, DiMain, DiNote, DiRow, DiSection } from "../proxmox/DiList";
import type { PBSTask } from "../pbsTabModel";
import { JOB_COLS, JOB_COLS_S, PBSLastRun } from "./PBSLastRun";
import { normalizePruneJobs, retentionParts, type PBSPruneJob } from "./pbsJobsModel";
import { newestTask, taskState } from "./pbsViewModel";
import { pbsAction, pbsErrorDetail, usePBSActions, usePBSResource } from "./usePBSData";
import { usePBSWords } from "./usePBSWords";

const EMPTY: PBSPruneJob[] = [];

export function PBSPruneJobs({ assetId, reloadKey, tasks }: { assetId: string; reloadKey: number; tasks: PBSTask[] }) {
  const { t, tk, schedule } = usePBSWords();
  const base = `/api/pbs/assets/${encodeURIComponent(assetId)}/prune-jobs`;
  const jobs = usePBSResource(base, normalizePruneJobs, EMPTY, reloadKey);
  const { busy, note, run } = usePBSActions();
  const { ask, dialog } = useDiConfirm();

  const keeps = (job: PBSPruneJob) => {
    const parts = retentionParts(job).map((part) => t(`keep.${part.kind}`, { count: part.count }));
    return parts.length > 0 ? parts.join(", ") : t("keep.none");
  };
  const nameOf = (job: PBSPruneJob) => (job.ns ? t("jobs.pruneNameNs", { store: job.store, ns: job.ns }) : t("jobs.pruneName", { store: job.store }));
  const act = (job: PBSPruneJob, action: "run" | "simulate" | "delete") => run(
    `${action}-${job.id}`,
    () => pbsAction(action === "delete" ? `${base}/${encodeURIComponent(job.id)}` : `${base}/${encodeURIComponent(job.id)}/${action}`, action === "delete" ? "DELETE" : "POST"),
    action === "delete" ? t("jobs.deleted", { name: nameOf(job) }) : t(`jobs.prune.${action}Started`, { store: job.store }),
    action === "delete" ? t("jobs.deleteFailed") : t("jobs.runFailed"),
    jobs.refresh,
  );

  return (
    <DiSection id="pbs-prune" title={t("jobs.prune.title")}>
      {jobs.error ? <DiNote tone="bad">{[t("jobs.loadFailed"), pbsErrorDetail(jobs.error)].filter(Boolean).join(" ")}</DiNote> : null}
      {note ? <DiNote tone={note.tone}>{note.text}</DiNote> : null}
      <DiList cols={JOB_COLS} colsS={JOB_COLS_S} hang label={t("jobs.prune.title")}>
        <DiCols>
          <span>{t("cols.job")}</span>
          <span className="r di-s">{t("cols.lastRun")}</span>
          <span />
        </DiCols>
        {jobs.data.length === 0 ? <DiEmpty title={jobs.loaded ? t("jobs.prune.none") : tk("loading")} /> : null}
        {jobs.data.map((job) => {
          const name = nameOf(job);
          const last = newestTask(tasks, "prune", job.store);
          return (
            <DiRow key={job.id} tone={last && taskState(last) === "failed" ? "bad" : ""}>
              <DiMain name={name} grey={<DiDots parts={[keeps(job), schedule(job.schedule), job.comment]} />} />
              <PBSLastRun task={last} className="di-s" />
              <DiRowMenu
                label={tk("actionsFor", { name })}
                items={[
                  { label: t("jobs.prune.simulate"), disabled: busy !== null, onSelect: () => void act(job, "simulate") },
                  {
                    label: t("jobs.runNowAsk"),
                    disabled: busy !== null,
                    onSelect: () => ask({
                      title: t("jobs.prune.askTitle", { store: job.store }),
                      message: t("jobs.prune.askBody", { keep: keeps(job) }),
                      confirmLabel: t("jobs.prune.askConfirm"),
                      onConfirm: () => act(job, "run"),
                    }),
                  },
                  { separator: true },
                  {
                    label: t("jobs.delete"),
                    danger: true,
                    disabled: busy !== null,
                    onSelect: () => ask({
                      title: t("jobs.ask.deleteTitle", { name }),
                      message: t("jobs.ask.deleteBody"),
                      confirmLabel: t("jobs.ask.delete"),
                      onConfirm: () => act(job, "delete"),
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

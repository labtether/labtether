"use client";

// Verification: each verify job with its datastore's state (last run, verified, unverified, failed); datastores
// without a job still show their state. A job's ⋯ runs it now or deletes it after a confirm.

import type { ReactNode } from "react";

import { DiRowMenu, useDiConfirm } from "../proxmox/DiControls";
import { DiCols, DiDots, DiEmpty, DiList, DiMain, DiNote, DiRow, DiSection, DiTime, type DiTone } from "../proxmox/DiList";
import { normalizePBSVerificationResponse, type PBSDatastoreVerification, type PBSVerificationResponse } from "../pbsTabModel";
import { JOB_COLS, JOB_COLS_S } from "./PBSLastRun";
import { normalizeVerifyJobs, type PBSVerifyJob } from "./pbsJobsModel";
import { pbsAction, pbsErrorDetail, usePBSActions, usePBSResource } from "./usePBSData";
import { usePBSWords } from "./usePBSWords";

const EMPTY_STATUS: PBSVerificationResponse = { datastores: [] };
const EMPTY_JOBS: PBSVerifyJob[] = [];

type Props = { assetId: string; reloadKey: number; right?: ReactNode };

export function PBSVerifyJobs({ assetId, reloadKey, right }: Props) {
  const { t, tk, when, schedule } = usePBSWords();
  const base = `/api/pbs/assets/${encodeURIComponent(assetId)}`;
  const status = usePBSResource(`${base}/verification`, normalizePBSVerificationResponse, EMPTY_STATUS, reloadKey);
  const jobs = usePBSResource(`${base}/verify-jobs`, normalizeVerifyJobs, EMPTY_JOBS, reloadKey);
  const { busy, note, run } = usePBSActions();
  const { ask, dialog } = useDiConfirm();

  const byStore = new Map(status.data.datastores.map((entry) => [entry.store, entry]));
  const covered = new Set(jobs.data.map((job) => job.store));
  const unscheduled = status.data.datastores.filter((entry) => !covered.has(entry.store));

  const refreshAll = () => {
    jobs.refresh();
    status.refresh();
  };
  const runJob = (job: PBSVerifyJob) => run(
    `run-${job.id}`,
    () => pbsAction(`${base}/verify-jobs/${encodeURIComponent(job.id)}/run`, "POST"),
    t("jobs.verifyStarted", { store: job.store }),
    t("jobs.runFailed"),
    refreshAll,
  );
  const deleteJob = (job: PBSVerifyJob) => run(
    `delete-${job.id}`,
    () => pbsAction(`${base}/verify-jobs/${encodeURIComponent(job.id)}`, "DELETE"),
    t("jobs.deleted", { name: t("jobs.verifyName", { store: job.store }) }),
    t("jobs.deleteFailed"),
    refreshAll,
  );

  const stateCell = (entry?: PBSDatastoreVerification) => {
    if (!entry) return <DiTime className="quiet di-s">—</DiTime>;
    return (
      <DiTime className="di-s">
        <DiDots parts={[
          entry.last_verify_time ? when(entry.last_verify_time) : tk("never"),
          t("jobs.verified", { count: entry.verified_count }),
          entry.unverified_count > 0 ? <span key="u" className="di-warn">{t("jobs.unverified", { count: entry.unverified_count })}</span> : null,
          entry.failed_count > 0 ? <span key="f" className="di-bad">{t("jobs.failed", { count: entry.failed_count })}</span> : null,
        ]} />
      </DiTime>
    );
  };
  const toneOf = (entry?: PBSDatastoreVerification): DiTone => (entry?.status === "bad" ? "bad" : entry?.status === "warn" ? "warn" : "");
  const errors = [status.error, jobs.error].filter((error): error is string => error !== null);

  return (
    <DiSection id="pbs-verify" title={t("jobs.verify")} right={right}>
      {errors.length > 0 ? (
        <DiNote tone="bad">{[t("jobs.loadFailed"), ...errors.map(pbsErrorDetail)].filter(Boolean).join(" ")}</DiNote>
      ) : null}
      {(status.data.warnings ?? []).map((warning) => <DiNote key={warning} tone="warn">{warning}</DiNote>)}
      {note ? <DiNote tone={note.tone}>{note.text}</DiNote> : null}
      <DiList cols={JOB_COLS} colsS={JOB_COLS_S} hang label={t("jobs.verify")}>
        <DiCols>
          <span>{t("cols.job")}</span>
          <span className="r di-s">{t("cols.lastRun")}</span>
          <span />
        </DiCols>
        {jobs.data.length === 0 && unscheduled.length === 0 ? (
          <DiEmpty title={jobs.loaded && status.loaded ? t("jobs.noVerify") : tk("loading")} />
        ) : null}
        {jobs.data.map((job) => {
          const entry = byStore.get(job.store);
          const name = t("jobs.verifyName", { store: job.store });
          return (
            <DiRow key={job.id} tone={toneOf(entry)}>
              <DiMain
                name={name}
                grey={<DiDots parts={[
                  schedule(job.schedule),
                  job.outdated_after != null ? t("jobs.reverifyAfter", { days: job.outdated_after }) : null,
                  job.comment,
                ]} />}
              />
              {stateCell(entry)}
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
        {unscheduled.map((entry) => (
          <DiRow key={`status-${entry.store}`} tone={toneOf(entry)}>
            <DiMain name={t("jobs.verifyName", { store: entry.store })} grey={t("schedule.none")} />
            {stateCell(entry)}
            <span className="di-more-slot" />
          </DiRow>
        ))}
      </DiList>
      {dialog}
    </DiSection>
  );
}

"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { PBSActionConfirmation } from "./PBSActionConfirmation";
import { pbsAction, pbsFetch } from "./usePBSData";

type Props = {
  assetId: string;
};

type PruneJob = {
  id: string;
  store: string;
  ns?: string;
  schedule?: string;
  comment?: string;
  keep_last?: number;
  keep_hourly?: number;
  keep_daily?: number;
  keep_weekly?: number;
  keep_monthly?: number;
  keep_yearly?: number;
};

type PruneJobsResponse = {
  jobs?: unknown[];
  error?: string;
};

function normalizePruneJobs(value: unknown): PruneJob[] {
  if (!value || typeof value !== "object") return [];
  const raw = value as Record<string, unknown>;
  const jobs = raw.jobs;
  if (!Array.isArray(jobs)) return [];
  return jobs.map((entry) => {
    const j = (entry && typeof entry === "object" ? entry : {}) as Record<string, unknown>;
    return {
      id: typeof j.id === "string" ? j.id : String(j.id ?? ""),
      store: typeof j.store === "string" ? j.store : "",
      ns: typeof j.ns === "string" ? j.ns : undefined,
      schedule: typeof j.schedule === "string" ? j.schedule : undefined,
      comment: typeof j.comment === "string" ? j.comment : undefined,
      keep_last: numericJobField(j, "keep_last", "keep-last"),
      keep_hourly: numericJobField(j, "keep_hourly", "keep-hourly"),
      keep_daily: numericJobField(j, "keep_daily", "keep-daily"),
      keep_weekly: numericJobField(j, "keep_weekly", "keep-weekly"),
      keep_monthly: numericJobField(j, "keep_monthly", "keep-monthly"),
      keep_yearly: numericJobField(j, "keep_yearly", "keep-yearly"),
    };
  });
}

function numericJobField(
  job: Record<string, unknown>,
  snakeCase: string,
  pbsName: string,
): number | undefined {
  const value = job[snakeCase] ?? job[pbsName];
  return typeof value === "number" ? value : undefined;
}

function retentionSummary(job: PruneJob): string {
  const parts: string[] = [];
  if (job.keep_last != null) parts.push(`last=${job.keep_last}`);
  if (job.keep_hourly != null) parts.push(`hourly=${job.keep_hourly}`);
  if (job.keep_daily != null) parts.push(`daily=${job.keep_daily}`);
  if (job.keep_weekly != null) parts.push(`weekly=${job.keep_weekly}`);
  if (job.keep_monthly != null) parts.push(`monthly=${job.keep_monthly}`);
  if (job.keep_yearly != null) parts.push(`yearly=${job.keep_yearly}`);
  return parts.length > 0 ? parts.join(", ") : "\u2014";
}

export function PBSPruneJobsTab({ assetId }: Props) {
  const [jobs, setJobs] = useState<PruneJob[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionSuccess, setActionSuccess] = useState<string | null>(null);
  const [confirmation, setConfirmation] = useState<{
    action: "run" | "simulate" | "delete";
    id: string;
  } | null>(null);

  const seqRef = useRef(0);
  const latestRef = useRef(0);
  const actionSeq = useRef(0);

  const fetchJobs = useCallback(async () => {
    const id = ++seqRef.current;
    latestRef.current = id;
    setLoading(true);
    setError(null);
    try {
      const data = await pbsFetch<PruneJobsResponse>(
        `/api/pbs/assets/${encodeURIComponent(assetId)}/prune-jobs`,
      );
      if (latestRef.current !== id) return;
      setJobs(normalizePruneJobs(data));
    } catch (err) {
      if (latestRef.current !== id) return;
      setError(err instanceof Error ? err.message : "failed to load prune jobs");
    } finally {
      if (latestRef.current === id) setLoading(false);
    }
  }, [assetId]);

  useEffect(() => {
    void fetchJobs();
  }, [fetchJobs]);

  const doRun = useCallback(
    async (jobId: string) => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionSuccess(null);
      setConfirmation(null);
      setActionInFlight(`run-${jobId}`);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/prune-jobs/${encodeURIComponent(jobId)}/run`,
          "POST",
        );
        if (actionSeq.current === seq) {
          setActionSuccess(`Prune job ${jobId} started.`);
          void fetchJobs();
        }
      } catch (err) {
        if (actionSeq.current === seq)
          setActionError(err instanceof Error ? err.message : "run failed");
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, fetchJobs],
  );

  const doSimulate = useCallback(
    async (jobId: string) => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionSuccess(null);
      setConfirmation(null);
      setActionInFlight(`simulate-${jobId}`);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/prune-jobs/${encodeURIComponent(jobId)}/simulate`,
          "POST",
        );
        if (actionSeq.current === seq) {
          setActionSuccess(`Prune job ${jobId} simulation started.`);
          void fetchJobs();
        }
      } catch (err) {
        if (actionSeq.current === seq)
          setActionError(err instanceof Error ? err.message : "simulate failed");
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, fetchJobs],
  );

  const doDelete = useCallback(
    async (jobId: string) => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionSuccess(null);
      setConfirmation(null);
      setActionInFlight(`delete-${jobId}`);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/prune-jobs/${encodeURIComponent(jobId)}`,
          "DELETE",
        );
        if (actionSeq.current === seq) {
          setActionSuccess(`Prune job ${jobId} deleted.`);
          void fetchJobs();
        }
      } catch (err) {
        if (actionSeq.current === seq)
          setActionError(err instanceof Error ? err.message : "delete failed");
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, fetchJobs],
  );

  return (
    <div className="space-y-4">
      {confirmation ? (
        <PBSActionConfirmation
          message={`Confirm ${confirmation.action} prune job ${confirmation.id}?`}
          confirmLabel={confirmation.action === "delete" ? "Confirm Delete" : "Confirm"}
          busy={actionInFlight !== null}
          onConfirm={() => {
            if (confirmation.action === "run") void doRun(confirmation.id);
            else if (confirmation.action === "simulate") void doSimulate(confirmation.id);
            else void doDelete(confirmation.id);
          }}
          onCancel={() => setConfirmation(null)}
        />
      ) : null}
      <Card>
      <div className="flex items-center justify-between mb-3 flex-wrap gap-2">
        <h2 className="text-sm font-medium text-[var(--text)]">Prune Jobs</h2>
        <Button size="sm" variant="ghost" onClick={() => void fetchJobs()} disabled={loading}>
          {loading ? "Loading..." : "Refresh"}
        </Button>
      </div>

      {actionError ? <p role="alert" className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}
      {actionSuccess ? <p role="status" className="mb-3 text-xs text-[var(--ok)]">{actionSuccess}</p> : null}

      {error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : jobs.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">
          {loading ? "Loading prune jobs..." : "No prune jobs configured."}
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">ID</th>
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Store</th>
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Schedule</th>
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Retention</th>
                <th className="py-1 px-2 text-right text-[var(--muted)] font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((job) => (
                <tr key={job.id} className="border-b border-[var(--line)] border-opacity-30">
                  <td className="py-2 px-2 text-[var(--text)] font-medium">{job.id}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{job.store}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{job.schedule || "\u2014"}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{retentionSummary(job)}</td>
                  <td className="py-2 px-2 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!!actionInFlight}
                        loading={actionInFlight === `run-${job.id}`}
                        onClick={() => setConfirmation({ action: "run", id: job.id })}
                      >
                        Run
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!!actionInFlight}
                        loading={actionInFlight === `simulate-${job.id}`}
                        onClick={() => setConfirmation({ action: "simulate", id: job.id })}
                      >
                        Simulate
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!!actionInFlight}
                        loading={actionInFlight === `delete-${job.id}`}
                        onClick={() => setConfirmation({ action: "delete", id: job.id })}
                      >
                        Delete
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      </Card>
    </div>
  );
}

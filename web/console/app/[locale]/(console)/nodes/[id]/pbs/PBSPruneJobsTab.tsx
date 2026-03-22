"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
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
      keep_last: typeof j.keep_last === "number" ? j.keep_last : undefined,
      keep_hourly: typeof j.keep_hourly === "number" ? j.keep_hourly : undefined,
      keep_daily: typeof j.keep_daily === "number" ? j.keep_daily : undefined,
      keep_weekly: typeof j.keep_weekly === "number" ? j.keep_weekly : undefined,
      keep_monthly: typeof j.keep_monthly === "number" ? j.keep_monthly : undefined,
      keep_yearly: typeof j.keep_yearly === "number" ? j.keep_yearly : undefined,
    };
  });
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
      setActionInFlight(`run-${jobId}`);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/prune-jobs/${encodeURIComponent(jobId)}/run`,
          "POST",
        );
        if (actionSeq.current === seq) void fetchJobs();
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
      setActionInFlight(`simulate-${jobId}`);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/prune-jobs/${encodeURIComponent(jobId)}/simulate`,
          "POST",
        );
        if (actionSeq.current === seq) void fetchJobs();
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
      setActionInFlight(`delete-${jobId}`);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/prune-jobs/${encodeURIComponent(jobId)}`,
          "DELETE",
        );
        if (actionSeq.current === seq) void fetchJobs();
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
    <Card>
      <div className="flex items-center justify-between mb-3 flex-wrap gap-2">
        <h2 className="text-sm font-medium text-[var(--text)]">Prune Jobs</h2>
        <Button size="sm" variant="ghost" onClick={() => void fetchJobs()} disabled={loading}>
          {loading ? "Loading..." : "Refresh"}
        </Button>
      </div>

      {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}

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
                        onClick={() => void doRun(job.id)}
                      >
                        Run
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!!actionInFlight}
                        loading={actionInFlight === `simulate-${job.id}`}
                        onClick={() => void doSimulate(job.id)}
                      >
                        Simulate
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!!actionInFlight}
                        loading={actionInFlight === `delete-${job.id}`}
                        onClick={() => void doDelete(job.id)}
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
  );
}

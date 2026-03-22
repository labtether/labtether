"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { pbsAction, pbsFetch } from "./usePBSData";

type Props = {
  assetId: string;
};

type SyncJob = {
  id: string;
  store: string;
  remote?: string;
  remote_store?: string;
  schedule?: string;
  comment?: string;
  remove_vanished?: boolean;
};

type SyncJobsResponse = {
  jobs?: unknown[];
  error?: string;
};

function normalizeSyncJobs(value: unknown): SyncJob[] {
  if (!value || typeof value !== "object") return [];
  const raw = value as Record<string, unknown>;
  const jobs = raw.jobs;
  if (!Array.isArray(jobs)) return [];
  return jobs.map((entry) => {
    const j = (entry && typeof entry === "object" ? entry : {}) as Record<string, unknown>;
    return {
      id: typeof j.id === "string" ? j.id : String(j.id ?? ""),
      store: typeof j.store === "string" ? j.store : "",
      remote: typeof j.remote === "string" ? j.remote : undefined,
      remote_store: typeof j.remote_store === "string" ? j.remote_store : undefined,
      schedule: typeof j.schedule === "string" ? j.schedule : undefined,
      comment: typeof j.comment === "string" ? j.comment : undefined,
      remove_vanished: j.remove_vanished === true,
    };
  });
}

export function PBSSyncJobsTab({ assetId }: Props) {
  const [jobs, setJobs] = useState<SyncJob[]>([]);
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
      const data = await pbsFetch<SyncJobsResponse>(
        `/api/pbs/assets/${encodeURIComponent(assetId)}/sync-jobs`,
      );
      if (latestRef.current !== id) return;
      setJobs(normalizeSyncJobs(data));
    } catch (err) {
      if (latestRef.current !== id) return;
      setError(err instanceof Error ? err.message : "failed to load sync jobs");
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
          `/api/pbs/assets/${encodeURIComponent(assetId)}/sync-jobs/${encodeURIComponent(jobId)}/run`,
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

  const doDelete = useCallback(
    async (jobId: string) => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`delete-${jobId}`);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/sync-jobs/${encodeURIComponent(jobId)}`,
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
        <h2 className="text-sm font-medium text-[var(--text)]">Sync Jobs</h2>
        <Button size="sm" variant="ghost" onClick={() => void fetchJobs()} disabled={loading}>
          {loading ? "Loading..." : "Refresh"}
        </Button>
      </div>

      {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}

      {error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : jobs.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">
          {loading ? "Loading sync jobs..." : "No sync jobs configured."}
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">ID</th>
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Store</th>
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Remote</th>
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Remote Store</th>
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Schedule</th>
                <th className="py-1 px-2 text-right text-[var(--muted)] font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((job) => (
                <tr key={job.id} className="border-b border-[var(--line)] border-opacity-30">
                  <td className="py-2 px-2 text-[var(--text)] font-medium">{job.id}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{job.store}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{job.remote || "\u2014"}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{job.remote_store || "\u2014"}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{job.schedule || "\u2014"}</td>
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

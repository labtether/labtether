"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { PBSVerificationCard } from "../PBSVerificationCard";
import { pbsAction, pbsFetch } from "./usePBSData";

type Props = {
  assetId: string;
};

type VerifyJob = {
  id: string;
  store: string;
  schedule?: string;
  comment?: string;
  ignore_verified?: boolean;
  outdated_after?: number;
};

type VerifyJobsResponse = {
  jobs?: unknown[];
  error?: string;
};

function normalizeVerifyJobs(value: unknown): VerifyJob[] {
  if (!value || typeof value !== "object") return [];
  const raw = value as Record<string, unknown>;
  const jobs = raw.jobs;
  if (!Array.isArray(jobs)) return [];
  return jobs.map((entry) => {
    const j = (entry && typeof entry === "object" ? entry : {}) as Record<string, unknown>;
    return {
      id: typeof j.id === "string" ? j.id : String(j.id ?? ""),
      store: typeof j.store === "string" ? j.store : "",
      schedule: typeof j.schedule === "string" ? j.schedule : undefined,
      comment: typeof j.comment === "string" ? j.comment : undefined,
      ignore_verified: j.ignore_verified === true,
      outdated_after: typeof j.outdated_after === "number" ? j.outdated_after : undefined,
    };
  });
}

export function PBSVerificationTab({ assetId }: Props) {
  const [jobs, setJobs] = useState<VerifyJob[]>([]);
  const [jobsLoading, setJobsLoading] = useState(false);
  const [jobsError, setJobsError] = useState<string | null>(null);
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const seqRef = useRef(0);
  const latestRef = useRef(0);
  const actionSeq = useRef(0);

  const fetchJobs = useCallback(async () => {
    const id = ++seqRef.current;
    latestRef.current = id;
    setJobsLoading(true);
    setJobsError(null);
    try {
      const data = await pbsFetch<VerifyJobsResponse>(
        `/api/pbs/assets/${encodeURIComponent(assetId)}/verify-jobs`,
      );
      if (latestRef.current !== id) return;
      setJobs(normalizeVerifyJobs(data));
    } catch (err) {
      if (latestRef.current !== id) return;
      setJobsError(err instanceof Error ? err.message : "failed to load verify jobs");
    } finally {
      if (latestRef.current === id) setJobsLoading(false);
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
          `/api/pbs/assets/${encodeURIComponent(assetId)}/verify-jobs/${encodeURIComponent(jobId)}/run`,
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
          `/api/pbs/assets/${encodeURIComponent(assetId)}/verify-jobs/${encodeURIComponent(jobId)}`,
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
    <div className="space-y-4">
      <PBSVerificationCard assetId={assetId} />

      <Card>
        <div className="flex items-center justify-between mb-3 flex-wrap gap-2">
          <h2 className="text-sm font-medium text-[var(--text)]">Verification Jobs</h2>
          <Button size="sm" variant="ghost" onClick={() => void fetchJobs()} disabled={jobsLoading}>
            {jobsLoading ? "Loading..." : "Refresh"}
          </Button>
        </div>

        {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}

        {jobsError ? (
          <p className="text-xs text-[var(--bad)]">{jobsError}</p>
        ) : jobs.length === 0 ? (
          <p className="text-xs text-[var(--muted)]">
            {jobsLoading ? "Loading jobs..." : "No verification jobs configured."}
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">ID</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Store</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Schedule</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Outdated After</th>
                  <th className="py-1 px-2 text-right text-[var(--muted)] font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {jobs.map((job) => (
                  <tr key={job.id} className="border-b border-[var(--line)] border-opacity-30">
                    <td className="py-2 px-2 text-[var(--text)] font-medium">{job.id}</td>
                    <td className="py-2 px-2 text-[var(--muted)]">{job.store}</td>
                    <td className="py-2 px-2 text-[var(--muted)]">{job.schedule || "\u2014"}</td>
                    <td className="py-2 px-2 text-[var(--muted)]">
                      {job.outdated_after != null ? `${job.outdated_after}d` : "\u2014"}
                    </td>
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
    </div>
  );
}

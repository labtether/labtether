import type { Asset, UpdateRun, UpdateRunResult } from "../../../console/models";
import { isDeviceTier } from "../../../console/taxonomy";

export type RunTone = "busy" | "bad" | "";

/** A run the hub has not finished: queued or running. */
export const isLive = (run: Pick<UpdateRun, "status">) => run.status === "queued" || run.status === "running";
/** A dot only for a live run (it moves) or a failed one; a finished good run needs none. */
export const runTone = (run: Pick<UpdateRun, "status">): RunTone => (run.status === "failed" ? "bad" : isLive(run) ? "busy" : "");

const time = (iso?: string) => {
  const ms = Date.parse(iso ?? "");
  return Number.isFinite(ms) ? ms : 0;
};

/** Newest first; the hub's order is not promised. */
export const newestFirst = (runs: UpdateRun[]) => [...runs].sort((a, b) => time(b.created_at) - time(a.created_at) || b.id.localeCompare(a.id));

/** Each plan's newest run, for the plan list's Last run column. */
export function lastRunByPlan(runs: UpdateRun[]): Map<string, UpdateRun> {
  const last = new Map<string, UpdateRun>();
  for (const run of newestFirst(runs)) if (!last.has(run.plan_id)) last.set(run.plan_id, run);
  return last;
}

/** What a device got in a plan's newest finished run, keyed by target. */
export function lastResults(runs: UpdateRun[], planId: string): Map<string, UpdateRunResult> {
  const run = newestFirst(runs).find((r) => r.plan_id === planId && !isLive(r) && r.results?.length);
  return new Map((run?.results ?? []).map((result) => [result.target, result]));
}

/** A run's per-device results counted: updated, failed and anything else the hub reported. */
export function tally(results: UpdateRunResult[] = []) {
  const count = { succeeded: 0, failed: 0, other: 0 };
  for (const r of results) {
    if (r.status === "succeeded") count.succeeded += 1;
    else if (r.status === "failed") count.failed += 1;
    else count.other += 1;
  }
  return count;
}

/** Devices the plan form offers: device-tier assets, A to Z, matching the filter by name, id or OS. */
/** OS package updates run on Linux; a device known to run another system cannot be part of a plan. */
function runsOSUpdates(asset: Asset): boolean {
  const platform = (asset.platform || asset.metadata?.platform || "").trim().toLowerCase();
  return platform === "" || platform === "linux";
}

export function pickableDevices(assets: Asset[], query: string): Asset[] {
  const q = query.trim().toLowerCase();
  return assets
    .filter((a) => isDeviceTier(a) && runsOSUpdates(a))
    .filter((a) => !q || [a.name, a.id, a.metadata?.os_pretty_name ?? ""].some((v) => v.toLowerCase().includes(q)))
    .sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id));
}

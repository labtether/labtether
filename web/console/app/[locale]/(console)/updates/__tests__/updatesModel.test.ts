import { describe, expect, it } from "vitest";
import type { UpdateRun } from "../../../../console/models";
import { lastResults, lastRunByPlan, tally } from "../updatesModel";

const run = (id: string, plan: string, status: string, at: string, results: UpdateRun["results"] = []): UpdateRun => ({
  id, plan_id: plan, plan_name: plan, actor_id: "owner", dry_run: false, status, results, created_at: at, updated_at: at,
});
const ok = (target: string) => ({ target, scope: "os_packages", status: "succeeded", summary: "Upgraded 3 packages" });
const bad = (target: string) => ({ target, scope: "os_packages", status: "failed", summary: "Could not get lock" });

describe("update runs per plan", () => {
  // The hub does not promise an order; the newest run must win either way.
  const runs = [
    run("r1", "weekly", "failed", "2026-10-28T03:00:00Z", [ok("nas01"), bad("docker-01")]),
    run("r3", "weekly", "running", "2026-10-31T03:00:00Z"),
    run("r2", "hosts", "succeeded", "2026-10-30T03:00:00Z", [ok("pve-01")]),
  ];

  it("shows each plan's newest run, even one still running", () => {
    const last = lastRunByPlan(runs);
    expect(last.get("weekly")?.id).toBe("r3");
    expect(last.get("hosts")?.id).toBe("r2");
  });

  it("takes device results from the newest finished run, not one still going", () => {
    const results = lastResults(runs, "weekly");
    expect(results.get("docker-01")?.status).toBe("failed");
    expect(results.get("nas01")?.status).toBe("succeeded");
    expect(lastResults(runs, "missing").size).toBe(0);
  });

  it("counts results the hub reports in any other state apart from done and failed", () => {
    expect(tally([ok("a"), bad("b"), { target: "c", scope: "os_packages", status: "skipped", summary: "" }])).toEqual({ succeeded: 1, failed: 1, other: 1 });
  });
});

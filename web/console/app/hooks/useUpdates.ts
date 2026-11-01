"use client";

import { useCallback } from "react";
import { useSlowStatus, useStatusControls, useStatusSettings } from "../contexts/StatusContext";
import { EXECUTABLE_UPDATE_SCOPES, parseUpdatePlanInput } from "./updatePlanValidation";

/** Why a save or run did not happen: a missing field, or the hub's own words when it refused. */
export type UpdateActionResult = { ok: true } | { ok: false; reason: "name" | "devices" | "hub"; detail?: string };

async function hubError(response: Response): Promise<string | undefined> {
  const body = (await response.json().catch(() => null)) as { error?: unknown } | null;
  return typeof body?.error === "string" && body.error.trim() ? body.error.trim() : undefined;
}

/** Update plans and their runs from the hub's status, and the two things you can do with them: save a plan, run one. */
export function useUpdates() {
  const status = useSlowStatus();
  const { fetchStatus } = useStatusControls();
  const { defaultActorID, defaultUpdateDryRun } = useStatusSettings();

  const createPlan = useCallback(async (input: { name: string; targets: string[]; dryRun: boolean }): Promise<UpdateActionResult> => {
    const name = input.name.trim();
    if (!name) return { ok: false, reason: "name" };
    let targets: string[];
    let scopes: string[];
    try {
      ({ targets, scopes } = parseUpdatePlanInput(input.targets.join(","), EXECUTABLE_UPDATE_SCOPES.join(",")));
    } catch {
      return { ok: false, reason: "devices" };
    }
    try {
      const response = await fetch("/api/updates/plans", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, targets, scopes, default_dry_run: input.dryRun }),
      });
      if (!response.ok) return { ok: false, reason: "hub", detail: await hubError(response) };
      await fetchStatus();
      return { ok: true };
    } catch (err) {
      return { ok: false, reason: "hub", detail: err instanceof Error ? err.message : undefined };
    }
  }, [fetchStatus]);

  const runPlan = useCallback(async (planId: string, dryRun: boolean): Promise<UpdateActionResult> => {
    try {
      const response = await fetch(`/api/updates/plans/${encodeURIComponent(planId)}/execute`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ actor_id: defaultActorID, dry_run: dryRun }),
      });
      if (!response.ok) return { ok: false, reason: "hub", detail: await hubError(response) };
      await fetchStatus();
      return { ok: true };
    } catch (err) {
      return { ok: false, reason: "hub", detail: err instanceof Error ? err.message : undefined };
    }
  }, [defaultActorID, fetchStatus]);

  return {
    plans: status?.updatePlans ?? [],
    runs: status?.updateRuns ?? [],
    loaded: status != null,
    defaultDryRun: defaultUpdateDryRun,
    createPlan,
    runPlan,
  };
}

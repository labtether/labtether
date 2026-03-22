"use client";

import { FormEvent, useState } from "react";
import { useSlowStatus, useStatusControls, useStatusSettings } from "../contexts/StatusContext";

export function useUpdates() {
  const status = useSlowStatus();
  const { fetchStatus } = useStatusControls();
  const { defaultActorID, defaultUpdateDryRun } = useStatusSettings();

  const updatePlans = status?.updatePlans ?? [];
  const updateRuns = status?.updateRuns ?? [];

  const [updatePlanName, setUpdatePlanName] = useState<string>("");
  const [updatePlanTargets, setUpdatePlanTargets] = useState<string>("");
  const [updatePlanScopes, setUpdatePlanScopes] = useState<string>("");
  const [updatePlanDryRun, setUpdatePlanDryRun] = useState<boolean>(defaultUpdateDryRun);
  const [updatePlanSubmitting, setUpdatePlanSubmitting] = useState(false);
  const [updateMessage, setUpdateMessage] = useState<string | null>(null);

  const createUpdatePlan = async (event: FormEvent) => {
    event.preventDefault();
    if (!updatePlanName.trim()) {
      setUpdateMessage("Plan name is required.");
      return;
    }

    const targets = updatePlanTargets
      .split(",")
      .map((entry) => entry.trim())
      .filter((entry) => entry.length > 0);
    const scopes = updatePlanScopes
      .split(",")
      .map((entry) => entry.trim())
      .filter((entry) => entry.length > 0);

    setUpdatePlanSubmitting(true);
    setUpdateMessage(null);
    try {
      const response = await fetch("/api/updates/plans", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: updatePlanName.trim(),
          targets,
          scopes,
          default_dry_run: updatePlanDryRun
        })
      });
      const result = (await response.json()) as { plan?: { id: string }; error?: string };
      if (!response.ok) {
        throw new Error(result.error || `update plan create failed: ${response.status}`);
      }
      setUpdateMessage(`Created plan ${result.plan?.id ?? "plan"}`);
      await fetchStatus();
    } catch (err) {
      setUpdateMessage(err instanceof Error ? err.message : "failed to create plan");
    } finally {
      setUpdatePlanSubmitting(false);
    }
  };

  const executeUpdatePlan = async (planId: string) => {
    setUpdateMessage(null);
    try {
      const response = await fetch(`/api/updates/plans/${encodeURIComponent(planId)}/execute`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ actor_id: defaultActorID, dry_run: updatePlanDryRun })
      });
      const result = (await response.json()) as { job_id?: string; error?: string };
      if (!response.ok) {
        throw new Error(result.error || `update execution failed: ${response.status}`);
      }
      setUpdateMessage(`Update run queued as ${result.job_id ?? "job"}`);
      await fetchStatus();
    } catch (err) {
      setUpdateMessage(err instanceof Error ? err.message : "failed to execute update plan");
    }
  };

  return {
    updatePlans,
    updateRuns,
    updatePlanName,
    setUpdatePlanName,
    updatePlanTargets,
    setUpdatePlanTargets,
    updatePlanScopes,
    setUpdatePlanScopes,
    updatePlanDryRun,
    setUpdatePlanDryRun,
    updatePlanSubmitting,
    updateMessage,
    createUpdatePlan,
    executeUpdatePlan
  };
}

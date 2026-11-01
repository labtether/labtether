import type { ActionRun } from "../../../console/models";
import type { SavedAction, SavedActionRun, SavedActionRunStep } from "../../../hooks/useSavedActions";

/** A saved action's run started from this page: running, finished (with each step's result) or refused. */
export type SessionRun =
  | { state: "running" }
  | { state: "done"; at: string; result: SavedActionRun; failed: number }
  | { state: "refused"; at: string; message: string };

/** Status decides success. An exit code is shown only when the device reported it; an unknown one is never 0 or 1. */
export function stepSucceeded(step: SavedActionRunStep): boolean {
  return !step.error && step.status === "succeeded";
}

export function failedSteps(run: SavedActionRun): number {
  return run.steps.filter((step) => !stepSucceeded(step)).length;
}

/** The devices an action runs on, in step order, each once. */
export function actionDevices(action: Pick<SavedAction, "steps">): string[] {
  return [...new Set(action.steps.map((step) => step.target).filter(Boolean))];
}

export type RunTone = "busy" | "bad" | "ok" | "wait";

/** A hub action run's tone (the hub's states are queued, running, succeeded and failed). */
export function runTone(run: Pick<ActionRun, "status">): RunTone {
  const status = run.status.toLowerCase();
  if (status === "running") return "busy";
  if (status === "failed") return "bad";
  if (status === "succeeded") return "ok";
  return "wait";
}

/** Saved actions sorted by name, as the list shows them. */
export function sortedActions(actions: SavedAction[], dir: "asc" | "desc"): SavedAction[] {
  const sign = dir === "asc" ? 1 : -1;
  return [...actions].sort((a, b) => sign * a.name.localeCompare(b.name, undefined, { sensitivity: "base" }));
}

import type { ToastActionOptions } from "../../contexts/ToastContext";
import { ensureString } from "../../lib/responseGuards";

type AddToast = (type: "success" | "error" | "info" | "warning", message: string, durationMs?: number, action?: ToastActionOptions) => void;

type CollectorSnapshot = {
  collector?: {
    id: string;
    asset_id?: string;
    last_status?: string;
    last_error?: string;
    last_collected_at?: string;
  };
  discovered?: number;
  error?: string;
};

type WaitForCollectorResult = {
  ok: boolean;
  discovered?: number;
  error?: string;
  warning?: string;
  /** The collector's own warning text, without the generic fallback in `warning`. */
  warningDetail?: string;
  /** The run did not finish in time and no poll error explains why. */
  timedOut?: boolean;
};

/** The `auth.addDevice.sync` translator for the sync toasts. */
export type CollectorSyncT = (key: string, values?: Record<string, string | number>) => string;

const RUN_TIMEOUT_MS = 90_000;
const POLL_INTERVAL_MS = 1_500;

export async function startCollectorRun(collectorID: string): Promise<number> {
  const startedAtMs = Date.now();
  const response = await fetch(`/api/settings/collectors/${encodeURIComponent(collectorID)}/run`, {
    method: "POST",
    signal: AbortSignal.timeout(20_000),
  });
  const payload = (await response.json().catch(() => null)) as { error?: string } | null;
  if (!response.ok) {
    throw new Error(payload?.error || `failed to start collector run (${response.status})`);
  }
  return startedAtMs;
}

async function getCollectorSnapshot(collectorID: string): Promise<CollectorSnapshot> {
  const response = await fetch(`/api/settings/collectors/${encodeURIComponent(collectorID)}`, {
    cache: "no-store",
    signal: AbortSignal.timeout(10_000),
  });
  const payload = (await response.json().catch(() => null)) as CollectorSnapshot | null;
  if (!response.ok) {
    throw new Error(payload?.error || `failed to load collector status (${response.status})`);
  }
  return payload ?? {};
}

function collectorRunCompleted(snapshot: CollectorSnapshot, startedAtMs: number): boolean {
  const status = ensureString(snapshot.collector?.last_status).trim().toLowerCase();
  if (!status || status === "running") return false;

  const lastCollectedAt = snapshot.collector?.last_collected_at;
  const lastCollectedAtMs = lastCollectedAt ? Date.parse(lastCollectedAt) : Number.NaN;
  if (!Number.isFinite(lastCollectedAtMs)) return false;

  return lastCollectedAtMs >= startedAtMs - 1_000;
}

export async function waitForCollectorRun(collectorID: string, startedAtMs: number): Promise<WaitForCollectorResult> {
  const deadline = Date.now() + RUN_TIMEOUT_MS;
  let lastError = "";

  while (Date.now() < deadline) {
    try {
      const snapshot = await getCollectorSnapshot(collectorID);
      const status = ensureString(snapshot.collector?.last_status).trim().toLowerCase();
      if (collectorRunCompleted(snapshot, startedAtMs)) {
        if (status === "ok") {
          return { ok: true, discovered: snapshot.discovered };
        }
        if (status === "partial") {
          const detail = snapshot.collector?.last_error?.trim() ?? "";
          return { ok: true, discovered: snapshot.discovered, warning: detail || "collector completed with warnings", warningDetail: detail };
        }
        const reason = snapshot.collector?.last_error?.trim() || `collector finished with status "${status}"`;
        return { ok: false, error: reason };
      }
    } catch (err) {
      lastError = err instanceof Error ? err.message : "failed to poll collector status";
    }

    await new Promise<void>((resolve) => {
      window.setTimeout(resolve, POLL_INTERVAL_MS);
    });
  }

  return {
    ok: false,
    error: lastError || "collector sync did not complete in time",
    timedOut: !lastError,
  };
}

/**
 * Starts a sync for a connector that was saved and reports start, finish and failure as toasts, with a Retry button
 * on failure. `name` is the system's product name ("Proxmox"); `t` is the `auth.addDevice.sync` translator.
 */
export function monitorCollectorRunWithRetry(
  collectorID: string,
  name: string,
  addToast: AddToast,
  t: CollectorSyncT,
) {
  const retry = { label: t("retry"), onClick: () => { void runSync(); } };
  const runSync = async () => {
    let startedAtMs = 0;
    try {
      startedAtMs = await startCollectorRun(collectorID);
      addToast("info", t("started", { name }));
    } catch (err) {
      const message = err instanceof Error && err.message ? err.message : t("noReason");
      addToast("warning", t("startFailed", { name, error: message }), 0, retry);
      return;
    }

    const result = await waitForCollectorRun(collectorID, startedAtMs);
    if (result.ok) {
      const found = Number.isFinite(result.discovered) ? t("found", { count: result.discovered ?? 0 }) : "";
      const partial = result.warning !== undefined;
      const summary = [t(partial ? "doneWarnings" : "done", { name }), found, partial ? result.warningDetail ?? "" : ""];
      addToast(partial ? "warning" : "success", summary.filter(Boolean).join(" "));
      return;
    }

    const failure = result.timedOut
      ? t("timedOut", { name })
      : t("failed", { name, error: result.error || t("noReason") });
    addToast("warning", failure, 0, retry);
  };

  void runSync();
}

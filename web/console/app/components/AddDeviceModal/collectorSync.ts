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
};

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
          const reason = snapshot.collector?.last_error?.trim() || "collector completed with warnings";
          return { ok: true, discovered: snapshot.discovered, warning: reason };
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
  };
}

export function monitorCollectorRunWithRetry(
  collectorID: string,
  connectorLabel: string,
  addToast: AddToast,
) {
  const runSync = async () => {
    let startedAtMs = 0;
    try {
      startedAtMs = await startCollectorRun(collectorID);
      addToast("info", `${connectorLabel} sync started.`);
    } catch (err) {
      const message = err instanceof Error ? err.message : `failed to start ${connectorLabel.toLowerCase()} sync`;
      addToast(
        "warning",
        `${connectorLabel} sync failed to start: ${message}`,
        0,
        { label: "Retry Sync", onClick: () => { void runSync(); } }
      );
      return;
    }

    const result = await waitForCollectorRun(collectorID, startedAtMs);
    if (result.ok) {
      const discoveredSuffix = Number.isFinite(result.discovered)
        ? ` ${result.discovered} assets discovered.`
        : "";
      if (result.warning) {
        addToast("warning", `${connectorLabel} sync completed with warnings.${discoveredSuffix} ${result.warning}`.trim());
        return;
      }
      addToast("success", `${connectorLabel} sync complete.${discoveredSuffix}`);
      return;
    }

    addToast(
      "warning",
      `${connectorLabel} sync failed: ${result.error || "unknown error"}`,
      0,
      { label: "Retry Sync", onClick: () => { void runSync(); } }
    );
  };

  void runSync();
}

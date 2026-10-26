"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { usePollingTask } from "../../../../../hooks/usePollingTask";
import { sanitizeErrorMessage } from "../../../../../lib/sanitizeErrorMessage";
import {
  normalizePBSAssetDetailsResponse,
  type PBSAssetDetailsResponse,
} from "../pbsTabModel";

// ---------------------------------------------------------------------------
// Generic fetch helper
// ---------------------------------------------------------------------------

export async function pbsFetch<T>(path: string): Promise<T> {
  const response = await fetch(path, { cache: "no-store" });
  const json = (await response.json().catch(() => null)) as { error?: string } | null;
  if (!response.ok) {
    const err = (json as { error?: string } | null)?.error ?? `request failed (${response.status})`;
    throw new Error(err);
  }
  return json as T;
}

export async function pbsAction(path: string, method = "POST", body?: unknown): Promise<unknown> {
  const response = await fetch(path, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!response.ok) {
    const json = (await response.json().catch(() => null)) as { error?: string } | null;
    throw new Error(json?.error ?? `action failed (${response.status})`);
  }
  return response.json().catch(() => null);
}

// ---------------------------------------------------------------------------
// usePBSDetails — polls every 30s, preserves sequence-ref pattern from PBSTab
// ---------------------------------------------------------------------------

type PBSDetailsState = {
  details: PBSAssetDetailsResponse | null;
  loading: boolean;
  error: string | null;
};

const PBS_DETAILS_ERROR_FALLBACK =
  "Unable to reach Proxmox Backup Server. Check that it is online and the connector settings are correct, then try again.";

function pbsDetailsErrorMessage(rawMessage: string): string {
  const raw = rawMessage.trim();
  const normalized = raw.toLowerCase();
  const isGenericTransportError =
    normalized === "failed to fetch" ||
    /^failed to load pbs details(?:\s*\(\d+\))?$/.test(normalized) ||
    /^request failed(?:\s*\(\d+\))?$/.test(normalized);
  return sanitizeErrorMessage(
    isGenericTransportError ? "" : raw,
    PBS_DETAILS_ERROR_FALLBACK,
  );
}

export function usePBSDetails(
  assetId: string,
  onManualRefreshSettled?: () => void | Promise<void>,
): PBSDetailsState & { refresh: () => void } {
  const [state, setState] = useState<PBSDetailsState>({
    details: null,
    loading: false,
    error: null,
  });

  const onManualRefreshSettledRef = useRef(onManualRefreshSettled);

  useEffect(() => {
    onManualRefreshSettledRef.current = onManualRefreshSettled;
  }, [onManualRefreshSettled]);

  const notifyManualRefreshSettled = useCallback(() => {
    try {
      void Promise.resolve(onManualRefreshSettledRef.current?.()).catch(() => undefined);
    } catch {
      // Parent status refresh failures must not replace valid PBS details or
      // the actionable upstream error from the details request itself.
    }
  }, []);

  const fetchDetails = useCallback(async (signal: AbortSignal, manual: boolean) => {
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const response = await fetch(`/api/pbs/assets/${encodeURIComponent(assetId)}/details`, {
        cache: "no-store", signal,
      });
      const payload = normalizePBSAssetDetailsResponse(await response.json().catch(() => null));
      if (!response.ok) {
        throw new Error(payload.error || `failed to load pbs details (${response.status})`);
      }
      if (signal.aborted) return;
      setState({ details: payload, loading: false, error: null });
    } catch (err) {
      if (signal.aborted) return;
      setState((previous) => ({
        // Keep the last successful inventory visible while clearly labeling
        // the refresh failure. A first-load failure still has no cached data.
        details: previous.details,
        loading: false,
        error: pbsDetailsErrorMessage(
          err instanceof Error ? err.message : "failed to load pbs details",
        ),
      }));
    }
    if (manual && !signal.aborted) notifyManualRefreshSettled();
  }, [assetId, notifyManualRefreshSettled]);

  const refresh = usePollingTask(fetchDetails, 30_000);

  return { ...state, refresh };
}

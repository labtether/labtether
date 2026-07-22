"use client";

import { useCallback, useEffect, useRef, useState } from "react";

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

  const seqRef = useRef(0);
  const latestRef = useRef(0);
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

  const fetchDetails = useCallback(async (notifyStatus = false) => {
    const id = ++seqRef.current;
    latestRef.current = id;
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const response = await fetch(`/api/pbs/assets/${encodeURIComponent(assetId)}/details`, {
        cache: "no-store",
      });
      const payload = normalizePBSAssetDetailsResponse(await response.json().catch(() => null));
      if (!response.ok) {
        throw new Error(payload.error || `failed to load pbs details (${response.status})`);
      }
      if (latestRef.current !== id) return;
      setState({ details: payload, loading: false, error: null });
      if (notifyStatus) {
        notifyManualRefreshSettled();
      }
    } catch (err) {
      if (latestRef.current !== id) return;
      setState((previous) => ({
        // Keep the last successful inventory visible while clearly labeling
        // the refresh failure. A first-load failure still has no cached data.
        details: previous.details,
        loading: false,
        error: pbsDetailsErrorMessage(
          err instanceof Error ? err.message : "failed to load pbs details",
        ),
      }));
      if (notifyStatus) {
        notifyManualRefreshSettled();
      }
    }
  }, [assetId, notifyManualRefreshSettled]);

  useEffect(() => {
    void fetchDetails();
    const interval = setInterval(() => void fetchDetails(), 30_000);
    return () => clearInterval(interval);
  }, [fetchDetails]);

  return { ...state, refresh: () => { void fetchDetails(true); } };
}

"use client";

import { useCallback, useEffect, useRef, useState } from "react";

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

export function usePBSDetails(assetId: string): PBSDetailsState & { refresh: () => void } {
  const [state, setState] = useState<PBSDetailsState>({
    details: null,
    loading: false,
    error: null,
  });

  const seqRef = useRef(0);
  const latestRef = useRef(0);

  const fetchDetails = useCallback(async () => {
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
    } catch (err) {
      if (latestRef.current !== id) return;
      setState({
        details: null,
        loading: false,
        error: err instanceof Error ? err.message : "failed to load pbs details",
      });
    }
  }, [assetId]);

  useEffect(() => {
    void fetchDetails();
    const interval = setInterval(() => void fetchDetails(), 30_000);
    return () => clearInterval(interval);
  }, [fetchDetails]);

  return { ...state, refresh: () => { void fetchDetails(); } };
}

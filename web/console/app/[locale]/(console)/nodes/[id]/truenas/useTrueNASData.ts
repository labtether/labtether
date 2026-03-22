"use client";

import { useCallback, useEffect, useRef, useState } from "react";

// ---------------------------------------------------------------------------
// Generic fetch helpers
// ---------------------------------------------------------------------------

export type TrueNASDataResponse<T> = {
  data: T;
  fetched_at?: string;
  warnings?: string[];
};

export async function truenasFetch<T>(path: string): Promise<T> {
  const response = await fetch(path, { cache: "no-store" });
  const json = (await response.json().catch(() => null)) as
    | TrueNASDataResponse<T>
    | { error?: string }
    | null;
  if (!response.ok) {
    const err =
      (json as { error?: string } | null)?.error ??
      `request failed (${response.status})`;
    throw new Error(err);
  }
  // Support both wrapped `{ data: ... }` and raw responses
  const wrapped = json as TrueNASDataResponse<T> | null;
  if (wrapped && "data" in wrapped) {
    return wrapped.data;
  }
  return json as T;
}

export async function truenasAction(
  path: string,
  method = "POST",
  body?: unknown,
): Promise<unknown> {
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
// Overview
// ---------------------------------------------------------------------------

export type TrueNASOverviewAlert = {
  level: string;
  message: string;
  dismissed: boolean;
};

export type TrueNASOverviewService = {
  name: string;
  running: boolean;
  enabled: boolean;
};

export type TrueNASOverviewData = {
  hostname?: string;
  version?: string;
  model?: string;
  uptime?: string;
  cpu_cores?: number;
  memory_bytes?: number;
  ecc_enabled?: boolean;
  storage_used_bytes?: number;
  storage_total_bytes?: number;
  services?: TrueNASOverviewService[];
  alerts?: TrueNASOverviewAlert[];
};

type OverviewState = {
  data: TrueNASOverviewData | null;
  loading: boolean;
  error: string | null;
};

export function useTrueNASOverview(
  assetId: string,
): OverviewState & { refresh: () => void } {
  const [state, setState] = useState<OverviewState>({
    data: null,
    loading: false,
    error: null,
  });
  const seqRef = useRef(0);
  const latestRef = useRef(0);

  const fetch_ = useCallback(async () => {
    const id = ++seqRef.current;
    latestRef.current = id;
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const data = await truenasFetch<TrueNASOverviewData>(
        `/api/truenas/assets/${encodeURIComponent(assetId)}/overview`,
      );
      if (latestRef.current !== id) return;
      setState({ data, loading: false, error: null });
    } catch (err) {
      if (latestRef.current !== id) return;
      setState({
        data: null,
        loading: false,
        error: err instanceof Error ? err.message : "failed to load overview",
      });
    }
  }, [assetId]);

  useEffect(() => {
    void fetch_();
    const interval = setInterval(() => {
      void fetch_();
    }, 30_000);
    return () => clearInterval(interval);
  }, [fetch_]);

  return { ...state, refresh: () => { void fetch_(); } };
}

// ---------------------------------------------------------------------------
// Generic list hook factory
// ---------------------------------------------------------------------------

type ListState<T> = {
  data: T[];
  loading: boolean;
  error: string | null;
};

export function useTrueNASList<T>(
  assetId: string,
  endpoint: string,
): ListState<T> & { refresh: () => void } {
  const [state, setState] = useState<ListState<T>>({
    data: [],
    loading: false,
    error: null,
  });
  const seqRef = useRef(0);
  const latestRef = useRef(0);

  const fetch_ = useCallback(async () => {
    const id = ++seqRef.current;
    latestRef.current = id;
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const data = await truenasFetch<T[]>(
        `/api/truenas/assets/${encodeURIComponent(assetId)}/${endpoint}`,
      );
      if (latestRef.current !== id) return;
      setState({ data: data ?? [], loading: false, error: null });
    } catch (err) {
      if (latestRef.current !== id) return;
      setState({
        data: [],
        loading: false,
        error: err instanceof Error ? err.message : `failed to load ${endpoint}`,
      });
    }
  }, [assetId, endpoint]);

  useEffect(() => {
    void fetch_();
  }, [fetch_]);

  return { ...state, refresh: () => { void fetch_(); } };
}

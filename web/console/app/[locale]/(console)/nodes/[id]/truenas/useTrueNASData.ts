"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { usePollingTask } from "../../../../../hooks/usePollingTask";
import { normalizeTrueNASListRows, normalizeTrueNASOverview } from "./normalizeTrueNASResponse";

// ---------------------------------------------------------------------------
// Generic fetch helpers
// ---------------------------------------------------------------------------

export type TrueNASDataResponse<T> = {
  data: T;
  fetched_at?: string;
  warnings?: string[];
};

export async function truenasFetch<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, { cache: "no-store", signal });
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
  if (wrapped && typeof wrapped === "object" && "data" in wrapped) {
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
  state?: string;
  running?: boolean;
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

  const fetchTask = useCallback(async (signal: AbortSignal) => {
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const payload = await truenasFetch<unknown>(
        `/api/truenas/assets/${encodeURIComponent(assetId)}/overview`, signal,
      );
      if (signal.aborted) return;
      const data = normalizeTrueNASOverview(payload);
      setState({ data, loading: false, error: null });
    } catch (err) {
      if (signal.aborted) return;
      setState({
        data: null,
        loading: false,
        error: err instanceof Error ? err.message : "failed to load overview",
      });
    }
  }, [assetId]);

  const refresh = usePollingTask(fetchTask, 30_000);

  return { ...state, refresh };
}

// ---------------------------------------------------------------------------
// Generic list hook factory
// ---------------------------------------------------------------------------

// The dedicated Hub endpoints return named collections, not a generic data key.
const trueNASListKeys = {
  pools: "pools",
  datasets: "datasets",
  snapshots: "snapshots",
  replication: "replications",
  vms: "vms",
  services: "services",
  disks: "disks",
  "shares/smb": "smb",
  "shares/nfs": "nfs",
} as const;

export type TrueNASListEndpoint = keyof typeof trueNASListKeys;

function decodeTrueNASList<T>(payload: unknown, endpoint: TrueNASListEndpoint): T[] {
  if (Array.isArray(payload)) return payload as T[];
  const key = trueNASListKeys[endpoint];
  const items = payload && typeof payload === "object"
    ? (payload as Record<string, unknown>)[key]
    : undefined;
  if (!Array.isArray(items)) {
    throw new Error(`invalid ${endpoint} response: expected ${key} list`);
  }
  return items as T[];
}

type ListState<T> = {
  data: T[];
  loading: boolean;
  error: string | null;
};

export function useTrueNASList<T>(
  assetId: string,
  endpoint: TrueNASListEndpoint,
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
      const payload = await truenasFetch<unknown>(
        `/api/truenas/assets/${encodeURIComponent(assetId)}/${endpoint}`,
      );
      const data = normalizeTrueNASListRows(endpoint, decodeTrueNASList<unknown>(payload, endpoint), payload) as T[];
      if (latestRef.current !== id) return;
      setState({ data, loading: false, error: null });
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

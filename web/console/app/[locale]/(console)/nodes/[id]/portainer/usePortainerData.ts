"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export type PortainerOverviewData = {
  version: string;
  endpoint: string;
  url: string;
  containers: {
    running: number;
    stopped: number;
    total: number;
  };
  stacks: {
    running: number;
    stopped: number;
    total: number;
  };
  images: {
    count: number;
  };
  volumes: {
    count: number;
  };
  networks: {
    count: number;
  };
};

export type PortainerDataResponse<T> = {
  data: T;
  fetched_at: string;
  warnings?: string[];
};

export async function portainerFetch<T>(path: string): Promise<T> {
  const response = await fetch(path, { cache: "no-store" });
  const json = (await response.json().catch(() => null)) as PortainerDataResponse<T> | { error?: string } | null;
  if (!response.ok) {
    const err = (json as { error?: string } | null)?.error ?? `request failed (${response.status})`;
    throw new Error(err);
  }
  return (json as PortainerDataResponse<T>).data;
}

export async function portainerAction(
  path: string,
  method: string,
  body?: unknown,
): Promise<void> {
  const response = await fetch(path, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!response.ok) {
    const json = (await response.json().catch(() => null)) as { error?: string } | null;
    throw new Error(json?.error ?? `action failed (${response.status})`);
  }
}

type OverviewState = {
  data: PortainerOverviewData | null;
  loading: boolean;
  error: string | null;
};

export function usePortainerOverview(assetId: string): OverviewState & { refresh: () => void } {
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
      const data = await portainerFetch<PortainerOverviewData>(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/overview`,
      );
      if (latestRef.current !== id) return;
      setState({ data, loading: false, error: null });
    } catch (err) {
      if (latestRef.current !== id) return;
      setState({ data: null, loading: false, error: err instanceof Error ? err.message : "failed to load overview" });
    }
  }, [assetId]);

  useEffect(() => {
    void fetch_();
    const interval = setInterval(() => { void fetch_(); }, 30_000);
    return () => clearInterval(interval);
  }, [fetch_]);

  return { ...state, refresh: () => { void fetch_(); } };
}

"use client";

import { useCallback, useState } from "react";
import { usePollingTask } from "../../../../../hooks/usePollingTask";

export type PortainerOverviewData = {
  server_version?: string;
  endpoint_name?: string;
  endpoint_url?: string;
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

export async function portainerFetch<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, { cache: "no-store", signal });
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

  const fetchTask = useCallback(async (signal: AbortSignal) => {
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const data = await portainerFetch<PortainerOverviewData>(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/overview`, signal,
      );
      if (signal.aborted) return;
      setState({ data, loading: false, error: null });
    } catch (err) {
      if (signal.aborted) return;
      setState({ data: null, loading: false, error: err instanceof Error ? err.message : "failed to load overview" });
    }
  }, [assetId]);

  const refresh = usePollingTask(fetchTask, 30_000);

  return { ...state, refresh };
}

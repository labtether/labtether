"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { usePollingTask } from "../../../../../hooks/usePollingTask";

// ---------------------------------------------------------------------------
// Generic fetch helpers
// ---------------------------------------------------------------------------

export async function proxmoxFetch<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, { cache: "no-store", signal });
  const json = (await response.json().catch(() => null)) as
    | { data?: T; error?: string }
    | null;
  if (!response.ok) {
    const err =
      (json as { error?: string } | null)?.error ??
      `request failed (${response.status})`;
    throw new Error(err);
  }
  if (json && typeof json === "object" && "data" in json) {
    return (json as { data: T }).data;
  }
  return json as T;
}

export async function proxmoxAction(
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
    const json = (await response.json().catch(() => null)) as
      | { error?: string }
      | null;
    throw new Error(json?.error ?? `action failed (${response.status})`);
  }
  return response.json().catch(() => null);
}

// ---------------------------------------------------------------------------
// Generic single-fetch hook
// ---------------------------------------------------------------------------

type FetchState<T> = {
  data: T | null;
  loading: boolean;
  error: string | null;
};

export function useProxmoxFetch<T>(
  path: string | null,
  pollMs?: number,
): FetchState<T> & { refresh: () => void } {
  const [state, setState] = useState<FetchState<T>>({
    data: null,
    loading: false,
    error: null,
  });

  const fetchTask = useCallback(async (signal: AbortSignal) => {
    if (!path) return;
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const data = await proxmoxFetch<T>(path, signal);
      if (signal.aborted) return;
      setState({ data, loading: false, error: null });
    } catch (err) {
      if (signal.aborted) return;
      setState({
        data: null,
        loading: false,
        error: err instanceof Error ? err.message : "failed to load data",
      });
    }
  }, [path]);

  const refresh = usePollingTask(fetchTask, pollMs ?? 0, path !== null);

  return { ...state, refresh };
}

// ---------------------------------------------------------------------------
// Generic list hook
// ---------------------------------------------------------------------------

type ListState<T> = {
  data: T[];
  loading: boolean;
  error: string | null;
};

export function useProxmoxList<T>(
  path: string | null,
): ListState<T> & { refresh: () => void } {
  const [state, setState] = useState<ListState<T>>({
    data: [],
    loading: false,
    error: null,
  });
  const seqRef = useRef(0);
  const latestRef = useRef(0);

  const fetch_ = useCallback(async () => {
    if (!path) return;
    const id = ++seqRef.current;
    latestRef.current = id;
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const data = await proxmoxFetch<T[]>(path);
      if (latestRef.current !== id) return;
      setState({ data: data ?? [], loading: false, error: null });
    } catch (err) {
      if (latestRef.current !== id) return;
      setState({
        data: [],
        loading: false,
        error: err instanceof Error ? err.message : "failed to load data",
      });
    }
  }, [path]);

  useEffect(() => {
    void fetch_();
  }, [fetch_]);

  return { ...state, refresh: () => { void fetch_(); } };
}

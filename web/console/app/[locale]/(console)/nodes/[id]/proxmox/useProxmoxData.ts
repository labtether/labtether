"use client";

import { useCallback, useEffect, useRef, useState } from "react";

// ---------------------------------------------------------------------------
// Generic fetch helpers
// ---------------------------------------------------------------------------

export async function proxmoxFetch<T>(path: string): Promise<T> {
  const response = await fetch(path, { cache: "no-store" });
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
  const seqRef = useRef(0);
  const latestRef = useRef(0);

  const fetch_ = useCallback(async () => {
    if (!path) return;
    const id = ++seqRef.current;
    latestRef.current = id;
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const data = await proxmoxFetch<T>(path);
      if (latestRef.current !== id) return;
      setState({ data, loading: false, error: null });
    } catch (err) {
      if (latestRef.current !== id) return;
      setState({
        data: null,
        loading: false,
        error: err instanceof Error ? err.message : "failed to load data",
      });
    }
  }, [path]);

  useEffect(() => {
    void fetch_();
    if (pollMs && pollMs > 0) {
      const interval = setInterval(() => { void fetch_(); }, pollMs);
      return () => clearInterval(interval);
    }
  }, [fetch_, pollMs]);

  return { ...state, refresh: () => { void fetch_(); } };
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

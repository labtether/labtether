"use client";

import { useCallback, useEffect, useState } from "react";

interface DisplayInfo {
  name: string;
  width: number;
  height: number;
  primary: boolean;
  offset_x: number;
  offset_y: number;
}

interface DisplayListState {
  displays: DisplayInfo[];
  loading: boolean;
  error: string | null;
}

function normalizeDisplayList(value: unknown): DisplayInfo[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((entry) => {
      if (!entry || typeof entry !== "object" || Array.isArray(entry)) {
        return null;
      }
      const raw = entry as Record<string, unknown>;
      return {
        name: typeof raw.name === "string" ? raw.name : "",
        width: typeof raw.width === "number" && Number.isFinite(raw.width) ? raw.width : 0,
        height: typeof raw.height === "number" && Number.isFinite(raw.height) ? raw.height : 0,
        primary: typeof raw.primary === "boolean" ? raw.primary : false,
        offset_x: typeof raw.offset_x === "number" && Number.isFinite(raw.offset_x) ? raw.offset_x : 0,
        offset_y: typeof raw.offset_y === "number" && Number.isFinite(raw.offset_y) ? raw.offset_y : 0,
      };
    })
    .filter((entry): entry is DisplayInfo => entry !== null);
}

export function useDisplayList(nodeId: string, enabled: boolean) {
  const [state, setState] = useState<DisplayListState>({
    displays: [],
    loading: false,
    error: null,
  });

  const refresh = useCallback(async () => {
    if (!nodeId || !enabled) return;
    setState((s) => ({ ...s, loading: true, error: null }));
    try {
      const res = await fetch(`/api/v1/nodes/${nodeId}/displays`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json().catch(() => null)) as { displays?: unknown; error?: unknown } | null;
      if (typeof data?.error === "string" && data.error !== "") throw new Error(data.error);
      setState({ displays: normalizeDisplayList(data?.displays), loading: false, error: null });
    } catch (err) {
      setState((s) => ({ ...s, loading: false, error: (err as Error).message }));
    }
  }, [nodeId, enabled]);

  // Fetch on mount/enable
  useEffect(() => {
    if (enabled) refresh();
  }, [enabled, refresh]);

  return { ...state, refresh };
}

export type { DisplayInfo };

"use client";

import { useEffect, useRef, useState } from "react";

// PortainerCapabilities mirrors the portainer capabilities backend response.
export type PortainerCapabilities = {
  tabs: string[];
  kind: string;
  can_exec: boolean;
  fetched_at: string;
};

// TrueNASCapabilities mirrors the truenas capabilities backend response.
export type TrueNASCapabilities = {
  tabs: string[];
  is_scale: boolean;
  has_apps: boolean;
  fetched_at: string;
};

// PBSCapabilities mirrors the pbs capabilities backend response.
export type PBSCapabilities = {
  tabs: string[];
  kind: string;
  fetched_at: string;
};

// ProxmoxCapabilities mirrors the proxmox capabilities backend response.
export type ProxmoxCapabilities = {
  tabs: string[];
  kind: string;
  has_ceph: boolean;
  has_ha: boolean;
  has_replication: boolean;
  fetched_at: string;
};

type CapabilitiesState<T> = {
  data: T | null;
  loading: boolean;
  error: string | null;
};

function useCapabilities<T>(platform: string, assetId: string, enabled: boolean): CapabilitiesState<T> {
  const [state, setState] = useState<CapabilitiesState<T>>({ data: null, loading: false, error: null });
  const requestRef = useRef(0);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    if (!enabled || !assetId) {
      abortRef.current?.abort();
      setState({ data: null, loading: false, error: null });
      return;
    }

    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    const requestID = ++requestRef.current;
    setState((prev) => ({ ...prev, loading: true, error: null }));

    void (async () => {
      try {
        const url = `/api/${platform}/assets/${encodeURIComponent(assetId)}/capabilities`;
        const response = await window.fetch(url, { cache: "no-store", signal: ctrl.signal });
        const json = (await response.json().catch(() => null)) as T | { error?: string } | null;
        if (requestRef.current !== requestID) {
          return;
        }
        if (!response.ok) {
          const msg = (json as { error?: string } | null)?.error ?? `capabilities request failed (${response.status})`;
          setState((prev) => ({ data: prev.data, loading: false, error: msg }));
          return;
        }
        setState({ data: json as T, loading: false, error: null });
      } catch (err) {
        if ((err as Error)?.name === "AbortError" || requestRef.current !== requestID) {
          return;
        }
        setState((prev) => ({
          data: prev.data,
          loading: false,
          error: (err as Error)?.message ?? "capabilities unavailable",
        }));
      }
    })();

    return () => {
      abortRef.current?.abort();
    };
  }, [platform, assetId, enabled]);

  return state;
}

export function usePortainerCapabilities(assetId: string, enabled = true) {
  return useCapabilities<PortainerCapabilities>("portainer", assetId, enabled);
}

export function useTrueNASCapabilities(assetId: string, enabled = true) {
  return useCapabilities<TrueNASCapabilities>("truenas", assetId, enabled);
}

export function usePBSCapabilities(assetId: string, enabled = true) {
  return useCapabilities<PBSCapabilities>("pbs", assetId, enabled);
}

export function useProxmoxCapabilities(assetId: string, enabled = true) {
  return useCapabilities<ProxmoxCapabilities>("proxmox", assetId, enabled);
}

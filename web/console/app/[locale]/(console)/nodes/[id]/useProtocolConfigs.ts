"use client";

import { useCallback, useEffect, useState } from "react";

export type ProtocolType = "ssh" | "telnet" | "vnc" | "rdp" | "ard";

export interface ProtocolConfig {
  id: string;
  asset_id: string;
  protocol: ProtocolType;
  host: string;
  port: number;
  username: string;
  credential_profile_id: string;
  enabled: boolean;
  last_tested_at: string | null;
  test_status: "untested" | "success" | "failed";
  test_error: string;
  config: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface TestResult {
  success: boolean;
  latency_ms: number;
  error: string | null;
}

type UseProtocolConfigsReturn = {
  protocols: ProtocolConfig[];
  loading: boolean;
  error: string | null;
  addProtocol: (data: Partial<ProtocolConfig>) => Promise<{ ok: boolean; error?: string }>;
  updateProtocol: (protocol: ProtocolType, data: Partial<ProtocolConfig>) => Promise<{ ok: boolean; error?: string }>;
  deleteProtocol: (protocol: ProtocolType) => Promise<{ ok: boolean; error?: string }>;
  testConnection: (protocol: ProtocolType) => Promise<TestResult>;
  pushHubKey: () => Promise<{ ok: boolean; error?: string }>;
  refetch: () => void;
};

export function useProtocolConfigs(assetId: string): UseProtocolConfigsReturn {
  const [protocols, setProtocols] = useState<ProtocolConfig[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fetchTick, setFetchTick] = useState(0);

  useEffect(() => {
    if (!assetId) return;
    const controller = new AbortController();
    setLoading(true);
    setError(null);

    const load = async () => {
      try {
        const res = await fetch(`/api/assets/${encodeURIComponent(assetId)}/protocols`, {
          cache: "no-store",
          signal: controller.signal,
        });
        if (!res.ok) {
          const payload = (await res.json().catch(() => ({}))) as { error?: string };
          setError(payload.error ?? `Failed to load protocols (${res.status})`);
          setProtocols([]);
          return;
        }
        const data = (await res.json()) as { protocols?: ProtocolConfig[] } | ProtocolConfig[];
        if (Array.isArray(data)) {
          setProtocols(data);
        } else {
          setProtocols(data.protocols ?? []);
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : "Failed to load protocols.");
        setProtocols([]);
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    };

    void load();
    return () => { controller.abort(); };
  }, [assetId, fetchTick]);

  const refetch = useCallback(() => {
    setFetchTick((n) => n + 1);
  }, []);

  const addProtocol = useCallback(async (data: Partial<ProtocolConfig>): Promise<{ ok: boolean; error?: string }> => {
    try {
      const res = await fetch(`/api/assets/${encodeURIComponent(assetId)}/protocols`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      const payload = (await res.json().catch(() => ({}))) as { error?: string };
      if (!res.ok) {
        return { ok: false, error: payload.error ?? `Failed to add protocol (${res.status})` };
      }
      setFetchTick((n) => n + 1);
      return { ok: true };
    } catch (err) {
      return { ok: false, error: err instanceof Error ? err.message : "Failed to add protocol." };
    }
  }, [assetId]);

  const updateProtocol = useCallback(async (protocol: ProtocolType, data: Partial<ProtocolConfig>): Promise<{ ok: boolean; error?: string }> => {
    try {
      const res = await fetch(`/api/assets/${encodeURIComponent(assetId)}/protocols/${encodeURIComponent(protocol)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      const payload = (await res.json().catch(() => ({}))) as { error?: string };
      if (!res.ok) {
        return { ok: false, error: payload.error ?? `Failed to update protocol (${res.status})` };
      }
      setFetchTick((n) => n + 1);
      return { ok: true };
    } catch (err) {
      return { ok: false, error: err instanceof Error ? err.message : "Failed to update protocol." };
    }
  }, [assetId]);

  const deleteProtocol = useCallback(async (protocol: ProtocolType): Promise<{ ok: boolean; error?: string }> => {
    try {
      const res = await fetch(`/api/assets/${encodeURIComponent(assetId)}/protocols/${encodeURIComponent(protocol)}`, {
        method: "DELETE",
      });
      if (!res.ok) {
        const payload = (await res.json().catch(() => ({}))) as { error?: string };
        return { ok: false, error: payload.error ?? `Failed to delete protocol (${res.status})` };
      }
      setFetchTick((n) => n + 1);
      return { ok: true };
    } catch (err) {
      return { ok: false, error: err instanceof Error ? err.message : "Failed to delete protocol." };
    }
  }, [assetId]);

  const testConnection = useCallback(async (protocol: ProtocolType): Promise<TestResult> => {
    try {
      const res = await fetch(
        `/api/assets/${encodeURIComponent(assetId)}/protocols/${encodeURIComponent(protocol)}/test`,
        { method: "POST" },
      );
      const payload = (await res.json().catch(() => ({}))) as Partial<TestResult>;
      if (!res.ok) {
        return { success: false, latency_ms: 0, error: (payload as { error?: string }).error ?? `Test failed (${res.status})` };
      }
      return {
        success: payload.success ?? false,
        latency_ms: payload.latency_ms ?? 0,
        error: payload.error ?? null,
      };
    } catch (err) {
      return { success: false, latency_ms: 0, error: err instanceof Error ? err.message : "Test failed." };
    }
  }, [assetId]);

  const pushHubKey = useCallback(async (): Promise<{ ok: boolean; error?: string }> => {
    try {
      const res = await fetch(`/api/assets/${encodeURIComponent(assetId)}/protocols/ssh/push-hub-key`, {
        method: "POST",
      });
      const payload = (await res.json().catch(() => ({}))) as { error?: string };
      if (!res.ok) {
        return { ok: false, error: payload.error ?? `Failed to push hub key (${res.status})` };
      }
      return { ok: true };
    } catch (err) {
      return { ok: false, error: err instanceof Error ? err.message : "Failed to push hub key." };
    }
  }, [assetId]);

  return {
    protocols,
    loading,
    error,
    addProtocol,
    updateProtocol,
    deleteProtocol,
    testConnection,
    pushHubKey,
    refetch,
  };
}

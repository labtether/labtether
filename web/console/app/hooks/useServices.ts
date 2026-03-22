"use client";
import { useCallback, useState } from "react";
import { ensureArray, ensureRecord, ensureString } from "../lib/responseGuards";

export type ServiceInfo = {
  name: string;
  description: string;
  active_state: string;
  sub_state: string;
  enabled: string;
  load_state: string;
};

export function useServices(assetId: string) {
  const [services, setServices] = useState<ServiceInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(`/api/services/${encodeURIComponent(assetId)}`, { cache: "no-store" });
      const data = ensureRecord(await res.json().catch(() => null));
      if (!res.ok) throw new Error(ensureString(data?.error) || `Failed (${res.status})`);
      setServices(ensureArray<ServiceInfo>(data?.services));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch services");
    } finally {
      setLoading(false);
    }
  }, [assetId]);

  const performAction = useCallback(async (service: string, action: string) => {
    try {
      const res = await fetch(`/api/services/${encodeURIComponent(assetId)}/${action}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ service }),
      });
      const data = ensureRecord(await res.json().catch(() => null));
      if (!res.ok) throw new Error(ensureString(data?.error) || `Failed (${res.status})`);
      await refresh();
      return data;
    } catch (err) {
      throw err;
    }
  }, [assetId, refresh]);

  return { services, loading, error, refresh, performAction };
}

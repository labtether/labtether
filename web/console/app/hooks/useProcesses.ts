"use client";
import { useCallback, useState } from "react";
import { ensureArray, ensureRecord, ensureString } from "../lib/responseGuards";

export type ProcessInfo = {
  pid: number;
  name: string;
  user: string;
  cpu_pct: number;
  mem_pct: number;
  mem_rss: number;
  command: string;
};

export function useProcesses(assetId: string) {
  const [processes, setProcesses] = useState<ProcessInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async (sort = "cpu", limit = 25) => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(
        `/api/processes/${encodeURIComponent(assetId)}?sort=${sort}&limit=${limit}`,
        { cache: "no-store" }
      );
      const data = ensureRecord(await res.json().catch(() => null));
      if (!res.ok) throw new Error(ensureString(data?.error) || `Failed (${res.status})`);
      setProcesses(ensureArray<ProcessInfo>(data?.processes));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch processes");
    } finally {
      setLoading(false);
    }
  }, [assetId]);

  return { processes, loading, error, refresh };
}

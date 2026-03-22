"use client";
import { useCallback, useState } from "react";
import { ensureArray, ensureRecord, ensureString } from "../lib/responseGuards";

export type CronEntryInfo = {
  source: string;
  schedule: string;
  command: string;
  user: string;
  next_run?: string;
  last_run?: string;
};

export function useCron(assetId: string) {
  const [entries, setEntries] = useState<CronEntryInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(`/api/cron/${encodeURIComponent(assetId)}`, { cache: "no-store" });
      const data = ensureRecord(await res.json().catch(() => null));
      if (!res.ok) {
        throw new Error(ensureString(data?.error) || `Failed (${res.status})`);
      }
      setEntries(ensureArray<CronEntryInfo>(data?.entries));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch cron and timer entries");
    } finally {
      setLoading(false);
    }
  }, [assetId]);

  return { entries, loading, error, refresh };
}

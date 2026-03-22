"use client";
import { useCallback, useState } from "react";
import { ensureArray, ensureRecord, ensureString } from "../lib/responseGuards";

export type DiskMountInfo = {
  device: string;
  mount_point: string;
  fs_type: string;
  total: number;
  used: number;
  available: number;
  use_pct: number;
};

export function useDisks(assetId: string) {
  const [mounts, setMounts] = useState<DiskMountInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(`/api/disks/${encodeURIComponent(assetId)}`, { cache: "no-store" });
      const data = ensureRecord(await res.json().catch(() => null));
      if (!res.ok) {
        throw new Error(ensureString(data?.error) || `Failed (${res.status})`);
      }
      setMounts(ensureArray<DiskMountInfo>(data?.mounts));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch disks");
    } finally {
      setLoading(false);
    }
  }, [assetId]);

  return { mounts, loading, error, refresh };
}

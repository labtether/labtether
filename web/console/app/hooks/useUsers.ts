"use client";
import { useCallback, useState } from "react";
import { ensureArray, ensureRecord, ensureString } from "../lib/responseGuards";

export type UserSessionInfo = {
  username: string;
  terminal: string;
  remote_host?: string;
  login_time: string;
};

export function useUsers(assetId: string) {
  const [sessions, setSessions] = useState<UserSessionInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(`/api/users/${encodeURIComponent(assetId)}`, { cache: "no-store" });
      const data = ensureRecord(await res.json().catch(() => null));
      if (!res.ok) {
        throw new Error(ensureString(data?.error) || `Failed (${res.status})`);
      }
      setSessions(ensureArray<UserSessionInfo>(data?.sessions));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch user sessions");
    } finally {
      setLoading(false);
    }
  }, [assetId]);

  return { sessions, loading, error, refresh };
}

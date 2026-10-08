"use client";

import { useCallback, useState } from "react";
import { safeJSON } from "./serviceRequest";

export function useServiceSync(host: string | undefined, setError: (error: string | null) => void) {
  const [syncing, setSyncing] = useState(false);
  const sync = useCallback(
    async (targetHost?: string) => {
      setSyncing(true);
      try {
        const params = new URLSearchParams();
        const hostValue = targetHost ?? host;
        if (hostValue) params.set("host", hostValue);
        const url = `/api/services/web/sync${params.toString() ? "?" + params.toString() : ""}`;

        const res = await fetch(url, {
          method: "POST",
          cache: "no-store",
        });
        if (!res.ok) {
          const payload = (await safeJSON(res)) as { error?: string } | null;
          throw new Error(payload?.error ?? `HTTP ${res.status}`);
        }
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to trigger service sync");
        throw err;
      } finally {
        setSyncing(false);
      }
    },
    [host, setError]
  );

  return { syncing, sync };
}

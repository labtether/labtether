"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { FileConnection } from "./fileConnectionsClient";
import { listFileConnections } from "./fileConnectionsClient";

// ---------------------------------------------------------------------------
// useFileConnections -- fetch saved file connections from API
// Uses direct fetch pattern with AbortController (matches useEdges.ts)
// ---------------------------------------------------------------------------

export function useFileConnections(): {
  connections: FileConnection[];
  loading: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const [connections, setConnections] = useState<FileConnection[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inFlightRef = useRef(false);
  const abortRef = useRef<AbortController | null>(null);

  const fetchConnections = useCallback(async () => {
    if (inFlightRef.current) return;
    inFlightRef.current = true;
    const controller = new AbortController();
    abortRef.current = controller;
    setLoading(true);
    setError(null);

    try {
      const result = await listFileConnections();
      // Only apply if this controller is still the active one.
      if (abortRef.current !== controller) return;
      setConnections(result);
      setError(null);
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      if (abortRef.current !== controller) return;
      setConnections([]);
      setError(err instanceof Error ? err.message : "Failed to load file connections.");
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      inFlightRef.current = false;
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void fetchConnections();
    return () => {
      abortRef.current?.abort();
    };
  }, [fetchConnections]);

  return { connections, loading, error, refetch: fetchConnections };
}

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { Proposal } from "../console/models";
import { ensureArray, ensureRecord } from "../lib/responseGuards";

// ---------------------------------------------------------------------------
// useProposals — fetch pending edge proposals from discovery
// ---------------------------------------------------------------------------

export function useProposals(): {
  proposals: Proposal[];
  loading: boolean;
  accept: (id: string) => Promise<void>;
  dismiss: (id: string) => Promise<void>;
  refetch: () => Promise<void>;
} {
  const [proposals, setProposals] = useState<Proposal[]>([]);
  const [loading, setLoading] = useState(true);
  const inFlightRef = useRef(false);
  const abortRef = useRef<AbortController | null>(null);

  const fetchProposals = useCallback(async () => {
    if (inFlightRef.current) return;
    inFlightRef.current = true;
    const controller = new AbortController();
    abortRef.current = controller;

    try {
      const res = await fetch("/api/v1/discovery/proposals", {
        cache: "no-store",
        signal: controller.signal,
      });
      if (res.ok) {
        const data = ensureRecord(await res.json().catch(() => null));
        setProposals(ensureArray<Proposal>(data?.proposals ?? data));
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      // Silently ignore — proposals list stays empty
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      inFlightRef.current = false;
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void fetchProposals();
    return () => {
      abortRef.current?.abort();
    };
  }, [fetchProposals]);

  const accept = useCallback(async (id: string) => {
    try {
      const res = await fetch(
        `/api/v1/discovery/proposals/${encodeURIComponent(id)}/accept`,
        { method: "POST" },
      );
      if (res.ok) {
        setProposals((prev) => prev.filter((p) => p.id !== id));
      }
    } catch {
      // Silently ignore — proposal stays in list for retry
    }
  }, []);

  const dismiss = useCallback(async (id: string) => {
    try {
      const res = await fetch(
        `/api/v1/discovery/proposals/${encodeURIComponent(id)}/dismiss`,
        { method: "POST" },
      );
      if (res.ok) {
        setProposals((prev) => prev.filter((p) => p.id !== id));
      }
    } catch {
      // Silently ignore — proposal stays in list for retry
    }
  }, []);

  return { proposals, loading, accept, dismiss, refetch: fetchProposals };
}

// ---------------------------------------------------------------------------
// triggerDiscovery — fire-and-forget mutation helper
// ---------------------------------------------------------------------------

export async function triggerDiscovery(): Promise<void> {
  const res = await fetch("/api/v1/discovery/run", { method: "POST" });
  if (!res.ok) {
    throw new Error(`triggerDiscovery failed: ${res.status}`);
  }
}

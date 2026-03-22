"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { LinkSuggestion } from "../console/models";
import { ensureArray, ensureRecord } from "../lib/responseGuards";

export function useLinkSuggestions() {
  const [suggestions, setSuggestions] = useState<LinkSuggestion[]>([]);
  const [loading, setLoading] = useState(true);
  const inFlightRef = useRef(false);
  const abortRef = useRef<AbortController | null>(null);

  const fetchSuggestions = useCallback(async () => {
    if (inFlightRef.current) return;
    inFlightRef.current = true;
    const controller = new AbortController();
    abortRef.current = controller;
    try {
      const res = await fetch("/api/links/suggestions", {
        cache: "no-store",
        signal: controller.signal,
      });
      if (res.ok) {
        const data = ensureRecord(await res.json().catch(() => null));
        const all = ensureArray<LinkSuggestion>(data?.suggestions ?? data);
        // Only show pending suggestions
        setSuggestions(all.filter((s) => s.status === "pending"));
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      // Silently ignore network errors — banner simply stays hidden
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      inFlightRef.current = false;
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void fetchSuggestions();
    return () => {
      abortRef.current?.abort();
    };
  }, [fetchSuggestions]);

  const accept = useCallback(async (id: string) => {
    try {
      const res = await fetch(`/api/links/suggestions/${encodeURIComponent(id)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status: "accepted" }),
      });
      if (res.ok) {
        setSuggestions((prev) => prev.filter((s) => s.id !== id));
      }
    } catch {
      // Silently ignore — suggestion stays in list for retry
    }
  }, []);

  const dismiss = useCallback(async (id: string) => {
    try {
      const res = await fetch(`/api/links/suggestions/${encodeURIComponent(id)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status: "dismissed" }),
      });
      if (res.ok) {
        setSuggestions((prev) => prev.filter((s) => s.id !== id));
      }
    } catch {
      // Silently ignore — suggestion stays in list for retry
    }
  }, []);

  return { suggestions, loading, accept, dismiss, refetch: fetchSuggestions };
}

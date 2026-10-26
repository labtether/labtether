"use client";

import { useEffect, useState } from "react";
import { useDocumentVisibility } from "./useDocumentVisibility";

/**
 * Measures connection latency by timing a lightweight fetch to the hub.
 * Updates every `intervalMs` while enabled and visible, with one request at a time.
 */
export function useLatency(enabled: boolean, intervalMs = 5000) {
  const [latencyMs, setLatencyMs] = useState<number | null>(null);
  const isVisible = useDocumentVisibility();

  useEffect(() => {
    if (!enabled) {
      setLatencyMs(null);
      return;
    }
    if (!isVisible) return;

    const controller = new AbortController();
    let inFlight = false;

    const measure = async () => {
      if (inFlight || controller.signal.aborted) return;
      inFlight = true;
      try {
        const start = performance.now();
        await fetch("/api/health", { method: "GET", cache: "no-store", signal: controller.signal });
        const rtt = Math.round(performance.now() - start);
        if (!controller.signal.aborted) setLatencyMs(rtt);
      } catch {
        if (!controller.signal.aborted) setLatencyMs(null);
      } finally {
        inFlight = false;
      }
    };

    void measure();
    const timer = setInterval(measure, intervalMs);

    return () => {
      clearInterval(timer);
      controller.abort();
    };
  }, [enabled, intervalMs, isVisible]);

  return latencyMs;
}

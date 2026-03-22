"use client";

import { useEffect, useRef, useState } from "react";

/**
 * Measures connection latency by timing a lightweight fetch to the hub.
 * Updates every `intervalMs` while `enabled` is true.
 */
export function useLatency(enabled: boolean, intervalMs = 5000) {
  const [latencyMs, setLatencyMs] = useState<number | null>(null);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (!enabled) {
      setLatencyMs(null);
      if (timerRef.current) clearInterval(timerRef.current);
      return;
    }

    const measure = async () => {
      try {
        const start = performance.now();
        await fetch("/api/health", { method: "GET", cache: "no-store" });
        const rtt = Math.round(performance.now() - start);
        setLatencyMs(rtt);
      } catch {
        setLatencyMs(null);
      }
    };

    void measure();
    timerRef.current = setInterval(measure, intervalMs);

    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [enabled, intervalMs]);

  return latencyMs;
}

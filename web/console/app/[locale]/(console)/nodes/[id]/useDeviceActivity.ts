"use client";

import { useEffect, useState } from "react";
import type { LogEvent } from "../../../../console/models";
import { ensureArray, ensureRecord } from "../../../../lib/responseGuards";

/** The device's newest stored log lines for Overview's Activity (the same query the Logs tab makes, five lines). */
export function useDeviceActivity(nodeId: string, enabled: boolean) {
  const [events, setEvents] = useState<LogEvent[]>([]);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    if (!enabled || !nodeId) return;
    const controller = new AbortController();
    const load = async () => {
      try {
        const params = new URLSearchParams({ asset_id: nodeId, limit: "5", window: "24h" });
        const res = await fetch(`/api/logs/query?${params.toString()}`, { cache: "no-store", signal: controller.signal });
        const data = ensureRecord(await res.json().catch(() => null));
        setEvents(res.ok ? ensureArray<LogEvent>(data?.events).slice(0, 5) : []);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setEvents([]);
      } finally {
        if (!controller.signal.aborted) setLoaded(true);
      }
    };
    void load();
    return () => controller.abort();
  }, [enabled, nodeId]);

  return { events, loaded };
}

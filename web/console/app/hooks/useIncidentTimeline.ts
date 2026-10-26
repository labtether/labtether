"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { IncidentEvent } from "../console/models";
import { decodeIncidentTimeline } from "../lib/incidentTimeline";

type TimelineState = {
  id: string | null;
  events: IncidentEvent[];
  nextCursor?: string;
  loading: boolean;
  error: boolean;
};

function emptyState(id: string | null): TimelineState {
  return { id, events: [], loading: Boolean(id), error: false };
}

export function useIncidentTimeline(id: string | null) {
  const [state, setState] = useState<TimelineState>(() => emptyState(id));
  const active = useRef<AbortController | null>(null);

  const load = useCallback(async (cursor?: string) => {
    if (!id) return;
    active.current?.abort();
    const controller = new AbortController();
    active.current = controller;
    const isCurrent = () => active.current === controller && !controller.signal.aborted;
    setState((current) => ({ ...(cursor && current.id === id ? current : emptyState(id)), loading: true, error: false }));
    try {
      const query = new URLSearchParams({ limit: "50" });
      if (cursor) query.set("cursor", cursor);
      const response = await fetch(`/api/incidents/${encodeURIComponent(id)}/events?${query}`, {
        cache: "no-store", signal: controller.signal,
      });
      if (!response.ok) throw new Error("Incident timeline unavailable");
      const page = decodeIncidentTimeline(await response.json(), id);
      if (page.next_cursor && page.next_cursor === cursor) throw new Error("Incident timeline cursor did not advance");
      if (!isCurrent()) return;
      setState((current) => {
        // A live row can reappear after its event time changes. Keep its latest
        // data without duplicating it in the visible history.
        const events = new Map((cursor && current.id === id ? current.events : []).map((event) => [event.id, event]));
        for (const event of page.events) events.set(event.id, event);
        return { id, events: [...events.values()], nextCursor: page.next_cursor, loading: false, error: false };
      });
    } catch {
      if (isCurrent()) setState((current) => ({ ...current, loading: false, error: true }));
    } finally {
      if (isCurrent()) active.current = null;
    }
  }, [id]);

  useEffect(() => {
    if (id) void load();
    else setState(emptyState(null));
    return () => { active.current?.abort(); active.current = null; };
  }, [id, load]);

  const visible = state.id === id ? state : emptyState(id);
  return {
    events: visible.events,
    loading: visible.loading,
    error: visible.error,
    hasMore: Boolean(visible.nextCursor),
    onLoadOlder: () => { if (!active.current && visible.nextCursor) void load(visible.nextCursor); },
    onRetry: () => { if (!active.current) void load(visible.nextCursor); },
    onRefresh: () => { if (!active.current) void load(); },
  };
}

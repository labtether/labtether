"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useDocumentVisibility } from "../../../../../hooks/useDocumentVisibility";
import { TrueNASEventsCard } from "../TrueNASEventsCard";
import {
  normalizeTrueNASEventsResponse,
} from "../truenasTabModel";
import type { TrueNASEvent } from "../truenasTabModel";

type Props = {
  assetId: string;
};

export function TrueNASEventsTab({ assetId }: Props) {
  const [events, setEvents] = useState<TrueNASEvent[]>([]);
  const [eventsLoading, setEventsLoading] = useState(false);
  const inFlightRef = useRef(false);
  const isDocumentVisible = useDocumentVisibility();

  const fetchEvents = useCallback(
    async (signal?: AbortSignal) => {
      if (inFlightRef.current) return;
      inFlightRef.current = true;
      setEventsLoading(true);
      try {
        const params = new URLSearchParams({ limit: "100", window: "12h" });
        const res = await fetch(
          `/api/truenas/assets/${encodeURIComponent(assetId)}/events?${params.toString()}`,
          { cache: "no-store", signal },
        );
        const payload = normalizeTrueNASEventsResponse(await res.json().catch(() => null));
        if (res.ok) {
          setEvents(payload.events ?? []);
        }
      } catch {
        // ignore transient polling failures
      } finally {
        inFlightRef.current = false;
        setEventsLoading(false);
      }
    },
    [assetId],
  );

  useEffect(() => {
    if (!isDocumentVisible) return;
    const controller = new AbortController();
    void fetchEvents(controller.signal);
    const interval = setInterval(() => {
      void fetchEvents(controller.signal);
    }, 10_000);
    return () => {
      controller.abort();
      clearInterval(interval);
    };
  }, [fetchEvents, isDocumentVisible]);

  const eventRows = useMemo(() => events.slice(0, 30), [events]);

  return (
    <TrueNASEventsCard
      events={eventRows}
      loading={eventsLoading}
      onRefresh={() => { void fetchEvents(); }}
    />
  );
}

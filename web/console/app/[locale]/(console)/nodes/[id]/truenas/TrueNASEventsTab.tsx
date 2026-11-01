"use client";

// Live TrueNAS events from the last 12 hours, newest first, polled every 10 s while the page is visible. A dot marks
// warnings and errors.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { useDocumentVisibility } from "../../../../../hooks/useDocumentVisibility";
import { DiEmpty, DiList, DiMain, DiRow, DiTime } from "../proxmox/DiList";
import { whenText } from "../proxmox/diFormat";
import { normalizeTrueNASEventsResponse, type TrueNASEvent } from "../truenasTabModel";
import { levelTone } from "./truenasModel";

export function TrueNASEventsTab({ assetId }: { assetId: string }) {
  const t = useTranslations("devices-truenas.events");
  const k = useTranslations("devices-proxmox.kit");
  const locale = useLocale();
  const [events, setEvents] = useState<TrueNASEvent[]>([]);
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const inFlightRef = useRef(false);
  const isDocumentVisible = useDocumentVisibility();

  const fetchEvents = useCallback(async (signal?: AbortSignal) => {
    if (inFlightRef.current) return;
    inFlightRef.current = true;
    setLoading(true);
    try {
      const params = new URLSearchParams({ limit: "100", window: "12h" });
      const res = await fetch(`/api/truenas/assets/${encodeURIComponent(assetId)}/events?${params.toString()}`, { cache: "no-store", signal });
      const payload = normalizeTrueNASEventsResponse(await res.json().catch(() => null));
      if (res.ok) {
        setEvents(payload.events ?? []);
        setLoaded(true);
      }
    } catch {
      // A missed poll keeps the last events; the next poll tries again.
    } finally {
      inFlightRef.current = false;
      setLoading(false);
    }
  }, [assetId]);

  useEffect(() => {
    if (!isDocumentVisible) return;
    const controller = new AbortController();
    void fetchEvents(controller.signal);
    const interval = setInterval(() => { void fetchEvents(controller.signal); }, 10_000);
    return () => {
      controller.abort();
      clearInterval(interval);
    };
  }, [fetchEvents, isDocumentVisible]);

  const rows = useMemo(() => events.slice(0, 30), [events]);

  return (
    <DiList hang label={t("label")} cols="minmax(0, 1fr) 96px" colsS="minmax(0, 1fr) 72px">
      {rows.length === 0 ? <DiEmpty title={loading && !loaded ? k("loading") : t("empty")} /> : null}
      {rows.map((event, index) => (
        <DiRow key={event.id || `${event.timestamp}-${index}`} className="di-tn-wrap" tone={levelTone(event.level)}>
          <DiMain name={event.message} plain />
          <DiTime>{whenText(event.timestamp, locale)}</DiTime>
        </DiRow>
      ))}
    </DiList>
  );
}

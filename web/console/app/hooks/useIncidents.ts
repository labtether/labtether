"use client";

import { useCallback, useEffect, useState } from "react";
import type { Incident, IncidentEvent } from "../console/models";
import { ensureArray, ensureRecord } from "../lib/responseGuards";

export function useIncidents() {
  const [incidents, setIncidents] = useState<Incident[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchIncidents = useCallback(async (signal?: AbortSignal) => {
    try {
      const res = await fetch("/api/incidents", { cache: "no-store", signal });
      if (res.ok) {
        const data = (await res.json().catch(() => null)) as unknown;
        const payload = ensureRecord(data);
        setIncidents(Array.isArray(data) ? data as Incident[] : ensureArray<Incident>(payload?.incidents));
      }
      setError(null);
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      setError(err instanceof Error ? err.message : "incidents unavailable");
    } finally {
      if (!signal?.aborted) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void fetchIncidents(controller.signal);
    return () => { controller.abort(); };
  }, [fetchIncidents]);

  const createIncident = useCallback(async (req: { title: string; severity: string; summary?: string }) => {
    const res = await fetch("/api/incidents", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    });
    if (!res.ok) throw new Error(`Failed to create incident: ${res.status}`);
    await fetchIncidents();
  }, [fetchIncidents]);

  const updateIncident = useCallback(async (id: string, req: Record<string, unknown>) => {
    const res = await fetch(`/api/incidents/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    });
    if (!res.ok) throw new Error(`Failed to update incident: ${res.status}`);
    await fetchIncidents();
  }, [fetchIncidents]);

  return {
    incidents,
    loading,
    error,
    refresh: fetchIncidents,
    createIncident,
    updateIncident,
  };
}

export function useIncidentDetail(id: string | null) {
  const [incident, setIncident] = useState<Incident | null>(null);
  const [events, setEvents] = useState<IncidentEvent[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!id) {
      setIncident(null);
      setEvents([]);
      return;
    }

    const controller = new AbortController();
    const load = async () => {
      setLoading(true);
      try {
        const [incRes, eventsRes] = await Promise.all([
          fetch(`/api/incidents/${encodeURIComponent(id)}`, { cache: "no-store", signal: controller.signal }),
          fetch(`/api/incidents/${encodeURIComponent(id)}/events`, { cache: "no-store", signal: controller.signal }),
        ]);

        if (incRes.ok) {
          const payload = (await incRes.json().catch(() => null)) as Incident | null;
          setIncident(payload);
        }
        if (eventsRes.ok) {
          const data = (await eventsRes.json().catch(() => null)) as unknown;
          const payload = ensureRecord(data);
          setEvents(Array.isArray(data) ? data as IncidentEvent[] : ensureArray<IncidentEvent>(payload?.events));
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        /* swallow other errors */
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    };

    void load();
    return () => { controller.abort(); };
  }, [id]);

  return { incident, events, loading };
}

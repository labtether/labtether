"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { Incident } from "../console/models";
import { extractIncident, upsertIncident } from "../lib/incidents";
import { ensureArray, ensureRecord } from "../lib/responseGuards";
import { useIncidentTimeline } from "./useIncidentTimeline";

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
    const created = extractIncident(await res.json().catch(() => null));
    if (!created) {
      await fetchIncidents();
      return null;
    }
    setIncidents((current) => upsertIncident(current, created));
    return created;
  }, [fetchIncidents]);

  const updateIncident = useCallback(async (id: string, req: Record<string, unknown>) => {
    const res = await fetch(`/api/incidents/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    });
    if (!res.ok) throw new Error(`Failed to update incident: ${res.status}`);
    const updated = extractIncident(await res.json().catch(() => null));
    if (!updated) {
      await fetchIncidents();
      throw new Error("Failed to parse updated incident");
    }
    setIncidents((current) => upsertIncident(current, updated));
    return updated;
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

type IncidentDetailState = { id: string | null; incident: Incident | null; loading: boolean; error: boolean };

function emptyDetail(id: string | null): IncidentDetailState {
  return { id, incident: null, loading: Boolean(id), error: false };
}

export function useIncidentDetail(id: string | null) {
  const [state, setState] = useState<IncidentDetailState>(() => emptyDetail(id));
  const active = useRef<AbortController | null>(null);
  const timeline = useIncidentTimeline(id);

  const loadDetail = useCallback(async () => {
    if (!id) return;
    active.current?.abort();
    const controller = new AbortController();
    active.current = controller;
    const isCurrent = () => active.current === controller && !controller.signal.aborted;
    setState(emptyDetail(id));
    try {
      const response = await fetch(`/api/incidents/${encodeURIComponent(id)}`, { cache: "no-store", signal: controller.signal });
      if (!response.ok) throw new Error("Incident unavailable");
      const incident = extractIncident(await response.json());
      if (!incident || incident.id !== id) throw new Error("Invalid incident detail");
      if (isCurrent()) setState({ id, incident, loading: false, error: false });
    } catch {
      if (isCurrent()) setState({ id, incident: null, loading: false, error: true });
    } finally {
      if (isCurrent()) active.current = null;
    }
  }, [id]);

  useEffect(() => {
    if (id) void loadDetail();
    else setState(emptyDetail(null));
    return () => { active.current?.abort(); active.current = null; };
  }, [id, loadDetail]);

  const replaceIncident = useCallback((nextIncident: Incident) => {
    setState((current) => current.id === nextIncident.id ? { ...current, incident: nextIncident } : current);
  }, []);

  const visible = state.id === id ? state : emptyDetail(id);
  return {
    incident: visible.incident, events: timeline.events, loading: visible.loading, error: visible.error,
    retryDetail: () => { if (!active.current) void loadDetail(); }, replaceIncident, timeline,
  };
}

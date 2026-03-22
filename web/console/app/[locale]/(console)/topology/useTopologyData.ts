"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import type {
  TopologyState, Zone, ZoneMember, TopologyConnection,
  Viewport, UnsortedResponse, Position, Size,
} from "./topologyCanvasTypes";

const API_BASE = "/api/topology";

export function useTopologyData() {
  const [topology, setTopology] = useState<TopologyState | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const viewportTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const fetchTopology = useCallback(async () => {
    abortRef.current?.abort();
    const ac = new AbortController();
    abortRef.current = ac;
    try {
      setIsLoading(true);
      const res = await fetch(API_BASE, { signal: ac.signal });
      if (!res.ok) throw new Error(`Failed to load topology: ${res.status}`);
      const json = await res.json();
      // Backend wraps in v2 envelope: { data: TopologyState }
      const data: TopologyState = json.data ?? json;
      setTopology(data);
      setError(null);
    } catch (e) {
      if (e instanceof DOMException && e.name === "AbortError") return;
      setError(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchTopology();
    return () => { abortRef.current?.abort(); };
  }, [fetchTopology]);

  // Clean up viewport debounce timer on unmount
  useEffect(() => {
    return () => { if (viewportTimerRef.current) clearTimeout(viewportTimerRef.current); };
  }, []);

  // --- Mutations ---

  const mutate = useCallback(async (
    path: string,
    method: string,
    body?: unknown,
  ): Promise<unknown> => {
    const res = await fetch(`${API_BASE}${path}`, {
      method,
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err.message || err.error || `Request failed: ${res.status}`);
    }
    const json = await res.json().catch(() => null);
    return json?.data ?? json;
  }, []);

  const createZone = useCallback(async (zone: Partial<Zone>): Promise<Zone> => {
    const result = await mutate("/zones", "POST", zone) as Zone;
    await fetchTopology();
    return result;
  }, [mutate, fetchTopology]);

  const updateZone = useCallback(async (id: string, updates: Partial<Zone>): Promise<void> => {
    await mutate(`/zones/${id}`, "PUT", updates);
    await fetchTopology();
  }, [mutate, fetchTopology]);

  const deleteZone = useCallback(async (id: string): Promise<void> => {
    await mutate(`/zones/${id}`, "DELETE");
    await fetchTopology();
  }, [mutate, fetchTopology]);

  const setMembers = useCallback(async (zoneID: string, members: ZoneMember[]): Promise<void> => {
    await mutate(`/zones/${zoneID}/members`, "PUT", { members });
    await fetchTopology();
  }, [mutate, fetchTopology]);

  const reorderZones = useCallback(async (updates: { zone_id: string; parent_zone_id?: string; sort_order: number }[]): Promise<void> => {
    await mutate("/zones/reorder", "PUT", { updates });
    await fetchTopology();
  }, [mutate, fetchTopology]);

  const createConnection = useCallback(async (conn: Partial<TopologyConnection>): Promise<TopologyConnection> => {
    const result = await mutate("/connections", "POST", conn) as TopologyConnection;
    await fetchTopology();
    return result;
  }, [mutate, fetchTopology]);

  const updateConnection = useCallback(async (id: string, updates: { relationship?: string; label?: string }): Promise<void> => {
    await mutate(`/connections/${id}`, "PUT", updates);
    await fetchTopology();
  }, [mutate, fetchTopology]);

  const deleteConnection = useCallback(async (id: string): Promise<void> => {
    await mutate(`/connections/${id}`, "DELETE");
    await fetchTopology();
  }, [mutate, fetchTopology]);

  const saveViewport = useCallback((viewport: Viewport) => {
    // Debounced — save after 500ms of no changes
    if (viewportTimerRef.current) clearTimeout(viewportTimerRef.current);
    viewportTimerRef.current = setTimeout(async () => {
      try {
        await mutate("/viewport", "PUT", viewport);
      } catch {
        // Viewport save failures are non-critical
      }
    }, 500);
  }, [mutate]);

  const dismissAsset = useCallback(async (assetID: string): Promise<void> => {
    await mutate("/dismiss", "POST", { asset_id: assetID });
    await fetchTopology();
  }, [mutate, fetchTopology]);

  const autoPlace = useCallback(async (): Promise<void> => {
    await mutate("/auto-place", "POST", {});
    await fetchTopology();
  }, [mutate, fetchTopology]);

  const resetTopology = useCallback(async (): Promise<void> => {
    await mutate("/reset", "POST", {});
    await fetchTopology();
  }, [mutate, fetchTopology]);

  return {
    topology,
    isLoading,
    error,
    refresh: fetchTopology,
    createZone,
    updateZone,
    deleteZone,
    setMembers,
    reorderZones,
    createConnection,
    updateConnection,
    deleteConnection,
    saveViewport,
    dismissAsset,
    autoPlace,
    resetTopology,
  };
}

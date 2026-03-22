"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Edge } from "../console/models";
import { ensureArray, ensureRecord } from "../lib/responseGuards";

// ---------------------------------------------------------------------------
// useEdges — fetch edges for a set of asset IDs
// ---------------------------------------------------------------------------

export function useEdges(assetIDs: string[]): {
  edges: Edge[];
  loading: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const [edges, setEdges] = useState<Edge[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inFlightRef = useRef(false);
  const abortRef = useRef<AbortController | null>(null);
  const assetIDsKey = useMemo(() => assetIDs.join("|"), [assetIDs]);
  const stableAssetIDs = useMemo(
    () => (assetIDsKey ? assetIDsKey.split("|").filter((id) => id.length > 0) : []),
    [assetIDsKey],
  );

  const fetchEdges = useCallback(async () => {
    if (stableAssetIDs.length === 0) {
      setEdges([]);
      setError(null);
      setLoading(false);
      return;
    }
    if (inFlightRef.current) return;
    inFlightRef.current = true;
    const controller = new AbortController();
    abortRef.current = controller;
    setLoading(true);
    setError(null);

    const params = new URLSearchParams();
    params.set("asset_ids", stableAssetIDs.join(","));
    params.set("limit", "5000");

    try {
      const res = await fetch(`/api/edges?${params.toString()}`, {
        cache: "no-store",
        signal: controller.signal,
      });
      if (!res.ok) {
        throw new Error(`failed ${res.status}`);
      }
      const data = ensureRecord(await res.json().catch(() => null));
      setEdges(ensureArray<Edge>(data?.edges ?? data));
      setError(null);
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      setEdges([]);
      setError(err instanceof Error ? err.message : "Failed to load edges.");
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      inFlightRef.current = false;
      setLoading(false);
    }
  }, [stableAssetIDs]);

  useEffect(() => {
    void fetchEdges();
    return () => {
      abortRef.current?.abort();
    };
  }, [fetchEdges]);

  return { edges, loading, error, refetch: fetchEdges };
}

// ---------------------------------------------------------------------------
// useEdgeTree — fetch descendant tree from a root asset
// ---------------------------------------------------------------------------

export function useEdgeTree(
  rootAssetID: string | null,
  depth?: number,
): {
  nodes: Array<{ asset_id: string; depth: number }>;
  loading: boolean;
  error: string | null;
} {
  const [nodes, setNodes] = useState<Array<{ asset_id: string; depth: number }>>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!rootAssetID) {
      setNodes([]);
      setError(null);
      setLoading(false);
      return;
    }

    let cancelled = false;
    setLoading(true);
    setError(null);

    const load = async () => {
      const params = new URLSearchParams({ root: rootAssetID });
      if (depth !== undefined) params.set("depth", String(depth));

      try {
        const res = await fetch(`/api/edges/tree?${params.toString()}`, {
          cache: "no-store",
        });
        if (!res.ok) throw new Error(`failed ${res.status}`);
        const data = ensureRecord(await res.json().catch(() => null));
        if (cancelled) return;
        setNodes(ensureArray<{ asset_id: string; depth: number }>(data?.nodes ?? data));
        setError(null);
      } catch (err) {
        if (cancelled) return;
        setNodes([]);
        setError(err instanceof Error ? err.message : "Failed to load edge tree.");
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, [rootAssetID, depth]);

  return { nodes, loading, error };
}

// ---------------------------------------------------------------------------
// useEdgeAncestors — fetch ancestor chain for an asset
// ---------------------------------------------------------------------------

export function useEdgeAncestors(assetID: string | null): {
  ancestors: Array<{ asset_id: string; depth: number }>;
  loading: boolean;
  error: string | null;
} {
  const [ancestors, setAncestors] = useState<Array<{ asset_id: string; depth: number }>>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!assetID) {
      setAncestors([]);
      setError(null);
      setLoading(false);
      return;
    }

    let cancelled = false;
    setLoading(true);
    setError(null);

    const load = async () => {
      const params = new URLSearchParams({ id: assetID });
      try {
        const res = await fetch(`/api/edges/ancestors?${params.toString()}`, {
          cache: "no-store",
        });
        if (!res.ok) throw new Error(`failed ${res.status}`);
        const data = ensureRecord(await res.json().catch(() => null));
        if (cancelled) return;
        setAncestors(ensureArray<{ asset_id: string; depth: number }>(data?.ancestors ?? data));
        setError(null);
      } catch (err) {
        if (cancelled) return;
        setAncestors([]);
        setError(err instanceof Error ? err.message : "Failed to load edge ancestors.");
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, [assetID]);

  return { ancestors, loading, error };
}

// ---------------------------------------------------------------------------
// Mutation helpers (not hooks)
// ---------------------------------------------------------------------------

export async function createEdge(req: {
  source_asset_id: string;
  target_asset_id: string;
  relationship_type: string;
  direction: string;
  criticality: string;
}): Promise<Edge> {
  const res = await fetch("/api/edges", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) {
    throw new Error(`createEdge failed: ${res.status}`);
  }
  return res.json() as Promise<Edge>;
}

export async function deleteEdge(id: string): Promise<void> {
  const res = await fetch(`/api/edges/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
  if (!res.ok) {
    throw new Error(`deleteEdge failed: ${res.status}`);
  }
}

export async function updateEdge(
  id: string,
  patch: { relationship_type?: string; criticality?: string },
): Promise<void> {
  const res = await fetch(`/api/edges/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
  });
  if (!res.ok) {
    throw new Error(`updateEdge failed: ${res.status}`);
  }
}

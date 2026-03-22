import { useEffect, useMemo, useState } from "react";
import { ensureArray, ensureRecord } from "../../../lib/responseGuards";

/**
 * Backward-compatible dependency type used throughout the topology views.
 * The Edge API returns a superset of these fields; consumers that only need
 * the core four fields can keep importing `AssetDependency`.
 */
export type AssetDependency = {
  id: string;
  source_asset_id: string;
  target_asset_id: string;
  relationship_type: string;
  /** Edge origin — present when fetched from the edges API. */
  origin?: "auto" | "manual" | "suggested" | "dismissed";
  /** Confidence score 0-100 — present when fetched from the edges API. */
  confidence?: number;
  /** Signal evidence map — present when fetched from the edges API. */
  match_signals?: Record<string, unknown>;
};

function normalizeEdge(raw: unknown, index: number): AssetDependency | null {
  if (!raw || typeof raw !== "object") return null;
  const candidate = raw as Partial<Record<string, unknown>>;
  if (
    typeof candidate.source_asset_id !== "string" ||
    typeof candidate.target_asset_id !== "string" ||
    typeof candidate.relationship_type !== "string"
  ) {
    return null;
  }

  const sourceAssetID = (candidate.source_asset_id as string).trim();
  const targetAssetID = (candidate.target_asset_id as string).trim();
  const relationshipType = (candidate.relationship_type as string).trim();
  if (!sourceAssetID || !targetAssetID || !relationshipType) return null;

  const providedID = typeof candidate.id === "string" ? candidate.id.trim() : "";
  const generatedID = `${sourceAssetID}->${targetAssetID}:${relationshipType}:${index}`;

  return {
    id: providedID || generatedID,
    source_asset_id: sourceAssetID,
    target_asset_id: targetAssetID,
    relationship_type: relationshipType,
    origin: typeof candidate.origin === "string"
      ? (candidate.origin as AssetDependency["origin"])
      : undefined,
    confidence: typeof candidate.confidence === "number"
      ? candidate.confidence
      : undefined,
    match_signals: candidate.match_signals && typeof candidate.match_signals === "object" && !Array.isArray(candidate.match_signals)
      ? (candidate.match_signals as Record<string, unknown>)
      : undefined,
  };
}

export function useAssetDependencies(assetIDs: string[]): {
  dependencies: AssetDependency[];
  relationshipsLoading: boolean;
  relationshipsError: string | null;
} {
  const [dependencies, setDependencies] = useState<AssetDependency[]>([]);
  const [relationshipsLoading, setRelationshipsLoading] = useState(false);
  const [relationshipsError, setRelationshipsError] = useState<string | null>(null);
  const assetIDsKey = useMemo(() => assetIDs.join("|"), [assetIDs]);
  const stableAssetIDs = useMemo(
    () => (assetIDsKey ? assetIDsKey.split("|").filter((id) => id.length > 0) : []),
    [assetIDsKey],
  );

  useEffect(() => {
    if (stableAssetIDs.length === 0) {
      setDependencies([]);
      setRelationshipsError(null);
      setRelationshipsLoading(false);
      return;
    }

    let cancelled = false;
    setRelationshipsLoading(true);
    setRelationshipsError(null);

    const load = async () => {
      const params = new URLSearchParams();
      params.set("asset_ids", stableAssetIDs.join(","));
      params.set("limit", "5000");

      try {
        const response = await fetch(`/api/edges?${params.toString()}`, {
          cache: "no-store",
        });
        if (!response.ok) {
          throw new Error(`failed ${response.status}`);
        }

        const data = ensureRecord(await response.json().catch(() => null));
        if (cancelled) return;

        const rawEdges = ensureArray<unknown>(data?.edges ?? data);
        const merged: AssetDependency[] = [];
        const seen = new Set<string>();
        for (let index = 0; index < rawEdges.length; index += 1) {
          const dep = normalizeEdge(rawEdges[index], index);
          if (!dep) continue;
          if (seen.has(dep.id)) continue;
          seen.add(dep.id);
          merged.push(dep);
        }
        setDependencies(merged);
        setRelationshipsError(null);
      } catch (error) {
        if (cancelled) return;
        setDependencies([]);
        setRelationshipsError(error instanceof Error ? error.message : "Failed to load relationship edges.");
      } finally {
        if (cancelled) return;
        setRelationshipsLoading(false);
      }
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, [stableAssetIDs]);

  return {
    dependencies,
    relationshipsLoading,
    relationshipsError,
  };
}

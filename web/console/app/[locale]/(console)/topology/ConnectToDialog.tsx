"use client";
import { useState, useMemo, useCallback, useEffect, useRef } from "react";
import type { RelationshipType, TopologyState } from "./topologyCanvasTypes";
import { RELATIONSHIP_TYPES } from "./topologyCanvasTypes";
import { inferRelationshipType } from "./topologySmartDefaults";
import { useFastStatus } from "../../../contexts/StatusContext";


interface ConnectToDialogProps {
  sourceAssetID: string;
  sourceAssetName: string;
  sourceAssetType: string;
  topology: TopologyState;
  onConnect: (targetAssetID: string, relationship: RelationshipType) => void;
  onClose: () => void;
}

export function ConnectToDialog({
  sourceAssetID,
  sourceAssetName,
  sourceAssetType,
  topology,
  onConnect,
  onClose,
}: ConnectToDialogProps) {
  const fastStatus = useFastStatus();
  const [query, setQuery] = useState("");
  const [selectedAssetID, setSelectedAssetID] = useState<string | null>(null);
  const [relationship, setRelationship] = useState<RelationshipType>("connected_to");
  const searchRef = useRef<HTMLInputElement>(null);

  // Auto-focus search on mount
  useEffect(() => {
    searchRef.current?.focus();
  }, []);

  // Close on Escape
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  // Build zone label lookup: assetID -> zone label
  const zoneLabelByAsset = useMemo(() => {
    const map = new Map<string, string>();
    for (const member of topology.members) {
      const zone = topology.zones.find((z) => z.id === member.zone_id);
      if (zone) map.set(member.asset_id, zone.label);
    }
    return map;
  }, [topology.members, topology.zones]);

  // All assets except the source
  const allAssets = useMemo(() => {
    return (fastStatus?.assets ?? []).filter((a) => a.id !== sourceAssetID);
  }, [fastStatus?.assets, sourceAssetID]);

  // Filtered by search query
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return allAssets;
    return allAssets.filter(
      (a) =>
        a.name.toLowerCase().includes(q) ||
        a.type.toLowerCase().includes(q) ||
        a.source.toLowerCase().includes(q) ||
        (zoneLabelByAsset.get(a.id) ?? "").toLowerCase().includes(q),
    );
  }, [allAssets, query, zoneLabelByAsset]);

  const selectedAsset = useMemo(
    () => allAssets.find((a) => a.id === selectedAssetID) ?? null,
    [allAssets, selectedAssetID],
  );

  // Auto-infer relationship when selection changes
  const handleSelectAsset = useCallback(
    (assetID: string, assetType: string) => {
      setSelectedAssetID(assetID);
      setRelationship(inferRelationshipType(sourceAssetType, assetType));
    },
    [sourceAssetType],
  );

  const handleConnect = useCallback(() => {
    if (!selectedAssetID) return;
    onConnect(selectedAssetID, relationship);
  }, [selectedAssetID, relationship, onConnect]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="flex w-[420px] max-h-[70vh] flex-col rounded-xl border border-[var(--panel-border)] bg-[var(--panel)] shadow-xl">
        {/* Header */}
        <div className="flex items-center justify-between px-4 pt-4 pb-3 border-b border-[var(--line)]">
          <h2 className="text-sm font-semibold text-[var(--text)]">
            Connect{" "}
            <span className="text-[var(--accent)]">{sourceAssetName}</span>{" "}
            to&hellip;
          </h2>
          <button
            onClick={onClose}
            className="rounded p-0.5 text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-fast)]"
            aria-label="Close"
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
              <path d="M1 1l12 12M13 1L1 13" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
            </svg>
          </button>
        </div>

        {/* Search */}
        <div className="px-4 pt-3 pb-2">
          <input
            ref={searchRef}
            type="text"
            placeholder="Search assets..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-1.5 text-xs text-[var(--text)] placeholder-[var(--muted)] outline-none focus:border-[var(--accent)] transition-colors duration-[var(--dur-fast)]"
          />
        </div>

        {/* Asset list */}
        <div className="flex-1 overflow-y-auto min-h-0">
          {filtered.length === 0 ? (
            <p className="px-4 py-3 text-xs text-[var(--muted)]">
              {query ? "No assets match your search." : "No other assets available."}
            </p>
          ) : (
            <ul>
              {filtered.map((asset) => {
                const zoneLabel = zoneLabelByAsset.get(asset.id);
                const isSelected = asset.id === selectedAssetID;
                return (
                  <li key={asset.id}>
                    <button
                      onClick={() => handleSelectAsset(asset.id, asset.type)}
                      className={`flex w-full items-center gap-2 px-3 py-2 text-xs transition-colors duration-[var(--dur-fast)] cursor-pointer ${
                        isSelected
                          ? "bg-[var(--accent)]/10 text-[var(--text)]"
                          : "text-[var(--text)] hover:bg-[var(--hover)]"
                      }`}
                    >
                      {/* Selection indicator */}
                      <span
                        className={`h-1.5 w-1.5 shrink-0 rounded-full transition-colors ${
                          isSelected ? "bg-[var(--accent)]" : "bg-transparent"
                        }`}
                      />
                      {/* Name */}
                      <span className="flex-1 truncate font-medium text-left">{asset.name}</span>
                      {/* Type badge */}
                      <span className="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium bg-[var(--surface)] text-[var(--muted)]">
                        {asset.type}
                      </span>
                      {/* Source */}
                      <span className="shrink-0 text-[var(--muted)] text-[10px]">{asset.source}</span>
                      {/* Zone */}
                      {zoneLabel && (
                        <span className="shrink-0 text-[var(--muted)] text-[10px] italic">{zoneLabel}</span>
                      )}
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>

        {/* Relationship selector — shown once an asset is selected */}
        {selectedAsset && (
          <div className="border-t border-[var(--line)] px-4 py-3 space-y-2">
            <p className="text-[10px] font-medium uppercase tracking-wide text-[var(--muted)]">
              Relationship
            </p>
            <div className="flex flex-wrap gap-1">
              {RELATIONSHIP_TYPES.map((rt) => (
                <button
                  key={rt.value}
                  onClick={() => setRelationship(rt.value)}
                  className={`rounded-md px-2.5 py-1 text-[11px] font-medium transition-colors duration-[var(--dur-fast)] ${
                    relationship === rt.value
                      ? "bg-[var(--accent)] text-[var(--accent-contrast)]"
                      : "bg-[var(--surface)] text-[var(--text)] hover:bg-[var(--hover)]"
                  }`}
                >
                  {rt.label}
                </button>
              ))}
            </div>
          </div>
        )}

        {/* Footer */}
        <div className="flex items-center justify-end gap-2 border-t border-[var(--line)] px-4 py-3">
          <button
            onClick={onClose}
            className="rounded-md px-3 py-1.5 text-xs text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-fast)]"
          >
            Cancel
          </button>
          <button
            onClick={handleConnect}
            disabled={!selectedAssetID}
            className="rounded-md bg-[var(--accent)] px-3 py-1.5 text-xs font-medium text-[var(--accent-contrast)] transition-opacity duration-[var(--dur-fast)] disabled:opacity-40 disabled:cursor-not-allowed hover:opacity-90"
          >
            Connect
          </button>
        </div>
      </div>
    </div>
  );
}

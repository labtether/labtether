"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useFastStatus } from "../../../contexts/StatusContext";
import type { PlacementSuggestion } from "./topologyCanvasTypes";

interface TopologyInboxProps {
  unsortedAssetIDs: string[];
  onAcceptSuggestion?: (assetID: string, zoneID: string) => void;
  onDismiss?: (assetID: string) => void;
  onAutoPlace?: () => void;
  onClose?: () => void;
}

export default function TopologyInbox({
  unsortedAssetIDs,
  onAcceptSuggestion,
  onDismiss,
  onAutoPlace,
  onClose,
}: TopologyInboxProps) {
  const fastStatus = useFastStatus();
  const [suggestions, setSuggestions] = useState<PlacementSuggestion[]>([]);
  const [loadingSuggestions, setLoadingSuggestions] = useState(false);
  const [autoPlacing, setAutoPlacing] = useState(false);
  const [dismissing, setDismissing] = useState<Set<string>>(new Set());
  const [accepting, setAccepting] = useState<Set<string>>(new Set());

  // Build an asset lookup map from status context
  const assetByID = useMemo(
    () => new Map((fastStatus?.assets ?? []).map((a) => [a.id, a])),
    [fastStatus?.assets],
  );

  // Re-fetch placement suggestions when unsorted assets change
  const unsortedKey = unsortedAssetIDs.join(",");
  useEffect(() => {
    let cancelled = false;
    async function fetchSuggestions() {
      setLoadingSuggestions(true);
      try {
        const res = await fetch("/api/topology/unsorted");
        if (!res.ok) return;
        const json = await res.json();
        const data = json.data ?? json;
        if (!cancelled && Array.isArray(data.suggestions)) {
          setSuggestions(data.suggestions);
        }
      } catch {
        // Non-critical; suggestions are advisory only
      } finally {
        if (!cancelled) setLoadingSuggestions(false);
      }
    }
    fetchSuggestions();
    return () => { cancelled = true; };
  }, [unsortedKey]);

  const suggestionByAssetID = useCallback(
    (assetID: string): PlacementSuggestion | null =>
      suggestions.find((s) => s.asset_id === assetID) ?? null,
    [suggestions],
  );

  const handleAccept = useCallback(
    async (assetID: string, zoneID: string) => {
      setAccepting((prev) => new Set(prev).add(assetID));
      try {
        onAcceptSuggestion?.(assetID, zoneID);
      } finally {
        setAccepting((prev) => {
          const next = new Set(prev);
          next.delete(assetID);
          return next;
        });
      }
    },
    [onAcceptSuggestion],
  );

  const handleDismiss = useCallback(
    async (assetID: string) => {
      setDismissing((prev) => new Set(prev).add(assetID));
      try {
        onDismiss?.(assetID);
      } finally {
        setDismissing((prev) => {
          const next = new Set(prev);
          next.delete(assetID);
          return next;
        });
      }
    },
    [onDismiss],
  );

  const handleAutoPlace = useCallback(async () => {
    setAutoPlacing(true);
    try {
      await onAutoPlace?.();
    } finally {
      setAutoPlacing(false);
    }
  }, [onAutoPlace]);

  const isEmpty = unsortedAssetIDs.length === 0;

  return (
    <div className="flex h-full flex-col border-l border-[var(--panel-border)] bg-[var(--panel)]">
      {/* Header */}
      <div className="flex shrink-0 items-center justify-between border-b border-[var(--panel-border)] px-4 py-3">
        <div className="flex items-center gap-2">
          <span className="text-sm font-semibold text-[var(--text)]">
            Unsorted Inbox
          </span>
          {unsortedAssetIDs.length > 0 && (
            <span className="flex h-5 min-w-5 items-center justify-center rounded-full bg-[var(--accent)] px-1.5 text-[10px] font-bold text-white">
              {unsortedAssetIDs.length}
            </span>
          )}
        </div>
        <button
          onClick={onClose}
          aria-label="Close inbox"
          className="flex h-6 w-6 items-center justify-center rounded text-[var(--muted)] transition-colors duration-[var(--dur-fast)] hover:bg-[var(--hover)] hover:text-[var(--text)]"
        >
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round">
            <path d="M2 2l10 10M12 2L2 12" />
          </svg>
        </button>
      </div>

      {/* Scrollable asset list */}
      <div className="min-h-0 flex-1 overflow-y-auto">
        {isEmpty ? (
          <div className="flex h-full flex-col items-center justify-center gap-2 px-4 py-12 text-center">
            <svg width="32" height="32" viewBox="0 0 32 32" fill="none" className="text-[var(--ok)] opacity-70">
              <circle cx="16" cy="16" r="13" stroke="currentColor" strokeWidth="2" />
              <path d="M10 16l4 4 8-8" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
            <p className="text-sm font-medium text-[var(--text)]">All assets are organized</p>
            <p className="text-xs text-[var(--muted)]">No unsorted assets remain.</p>
          </div>
        ) : (
          <ul className="divide-y divide-[var(--panel-border)]">
            {unsortedAssetIDs.map((assetID) => {
              const asset = assetByID.get(assetID);
              const suggestion = suggestionByAssetID(assetID);
              const isAccepting = accepting.has(assetID);
              const isDismissing = dismissing.has(assetID);

              return (
                <li key={assetID} className="flex flex-col gap-2 px-4 py-3 transition-colors duration-[var(--dur-fast)] hover:bg-[var(--hover)]">
                  {/* Asset identity row */}
                  <div className="flex items-center gap-2">
                    {/* Status dot */}
                    <span
                      className={`h-2 w-2 shrink-0 rounded-full ${
                        asset?.status === "ok"
                          ? "bg-[var(--ok)]"
                          : asset?.status === "error" || asset?.status === "bad"
                          ? "bg-[var(--bad)]"
                          : "bg-[var(--muted)]"
                      }`}
                    />
                    {/* Name */}
                    <span className="min-w-0 flex-1 truncate text-sm text-[var(--text)]">
                      {asset?.name ?? assetID}
                    </span>
                    {/* Type badge */}
                    {asset?.type && (
                      <span className="shrink-0 rounded bg-[var(--hover)] px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-[var(--muted)]">
                        {asset.type}
                      </span>
                    )}
                  </div>

                  {/* Suggestion + actions row */}
                  <div className="flex items-center gap-1.5">
                    {suggestion?.zone_id && suggestion.zone_label && !loadingSuggestions ? (
                      <>
                        <span className="min-w-0 flex-1 truncate text-xs text-[var(--muted)]">
                          Suggest:{" "}
                          <span className="font-medium text-[var(--text)]">{suggestion.zone_label}</span>
                        </span>
                        <button
                          disabled={isAccepting || isDismissing}
                          onClick={() => handleAccept(assetID, suggestion.zone_id!)}
                          className="shrink-0 rounded bg-[var(--accent)] px-2 py-0.5 text-[11px] font-medium text-white transition-opacity duration-[var(--dur-fast)] disabled:opacity-50 hover:opacity-90"
                        >
                          {isAccepting ? "..." : "Accept"}
                        </button>
                      </>
                    ) : loadingSuggestions ? (
                      <span className="flex-1 text-xs text-[var(--muted)]">Loading suggestion…</span>
                    ) : (
                      <span className="flex-1 text-xs text-[var(--muted)]">No suggestion</span>
                    )}
                    {/* Dismiss */}
                    <button
                      disabled={isDismissing || isAccepting}
                      onClick={() => handleDismiss(assetID)}
                      aria-label={`Dismiss ${asset?.name ?? assetID}`}
                      className="shrink-0 flex h-5 w-5 items-center justify-center rounded text-[var(--muted)] transition-colors duration-[var(--dur-fast)] disabled:opacity-40 hover:bg-[var(--hover)] hover:text-[var(--bad)]"
                    >
                      <svg width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round">
                        <path d="M1.5 1.5l7 7M8.5 1.5l-7 7" />
                      </svg>
                    </button>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      {/* Footer — Auto-place all */}
      {!isEmpty && (
        <div className="shrink-0 border-t border-[var(--panel-border)] px-4 py-3">
          <button
            disabled={autoPlacing}
            onClick={handleAutoPlace}
            className="w-full rounded-md bg-[var(--hover)] py-1.5 text-xs font-medium text-[var(--text)] transition-colors duration-[var(--dur-fast)] disabled:opacity-50 hover:bg-[var(--surface)]"
          >
            {autoPlacing ? "Placing…" : "Auto-place all"}
          </button>
        </div>
      )}
    </div>
  );
}

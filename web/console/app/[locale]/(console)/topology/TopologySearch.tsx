"use client";

import { useState, useMemo, useCallback, useRef, useEffect, type KeyboardEvent } from "react";
import { Search } from "lucide-react";
import { useFastStatus } from "../../../contexts/StatusContext";
import { isHiddenAsset, assetTypeIcon, friendlyTypeLabel } from "../../../console/taxonomy";
import { freshnessColor } from "./topologyUtils";

interface TopologySearchProps {
  onSelectResult: (assetID: string) => void;
  onClose: () => void;
}

export function TopologySearch({ onSelectResult, onClose }: TopologySearchProps) {
  const status = useFastStatus();
  const [query, setQuery] = useState("");
  const [highlightIndex, setHighlightIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const allAssets = useMemo(
    () => (status?.assets ?? []).filter((asset) => !isHiddenAsset(asset)),
    [status?.assets],
  );

  const results = useMemo(() => {
    const trimmed = query.trim().toLowerCase();
    if (!trimmed) return [];
    return allAssets
      .filter((asset) => asset.name.toLowerCase().includes(trimmed))
      .slice(0, 10);
  }, [allAssets, query]);

  useEffect(() => {
    setHighlightIndex(0);
  }, [results]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      if (e.key === "Escape") {
        onClose();
        return;
      }
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setHighlightIndex((i) => Math.min(i + 1, results.length - 1));
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setHighlightIndex((i) => Math.max(i - 1, 0));
        return;
      }
      if (e.key === "Enter") {
        const selected = results[highlightIndex];
        if (selected) {
          onSelectResult(selected.id);
        }
      }
    },
    [results, highlightIndex, onSelectResult, onClose],
  );

  return (
    <div className="flex flex-col">
      <div className="flex items-center gap-2 bg-[var(--panel)] border border-[var(--panel-border)] rounded-lg shadow-lg w-80 px-3 py-2">
        <Search size={13} className="shrink-0 text-[var(--muted)]" />
        <input
          ref={inputRef}
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Find asset on canvas…"
          className="flex-1 bg-transparent text-xs text-[var(--text)] placeholder:text-[var(--muted)] outline-none"
        />
      </div>

      {results.length > 0 && (
        <div className="mt-1 w-80 rounded-lg border border-[var(--panel-border)] bg-[var(--panel)] shadow-lg overflow-hidden">
          {results.map((asset, idx) => {
            const Icon = assetTypeIcon(asset.type);
            const colors = freshnessColor(asset);
            const isHighlighted = idx === highlightIndex;
            return (
              <button
                key={asset.id}
                type="button"
                className={`flex w-full items-center gap-2 px-3 py-1.5 text-xs text-left transition-colors ${
                  isHighlighted ? "bg-[var(--hover)]" : ""
                } hover:bg-[var(--hover)]`}
                onMouseEnter={() => setHighlightIndex(idx)}
                onClick={() => onSelectResult(asset.id)}
              >
                <Icon size={11} className="shrink-0 text-[var(--muted)]" />
                <span className="min-w-0 flex-1 truncate text-[var(--text)]">{asset.name}</span>
                <span className="shrink-0 text-[10px] text-[var(--muted)]">
                  {friendlyTypeLabel(asset.type)}
                </span>
                <span
                  className={`inline-block h-[6px] w-[6px] shrink-0 rounded-full ${colors.dot}`}
                />
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

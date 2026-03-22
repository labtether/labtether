"use client";

import { useState, useEffect, useRef, useCallback, useMemo } from "react";
import type { TerminalSnippet } from "../../hooks/useTerminalSnippets";

interface SnippetPickerProps {
  snippets: TerminalSnippet[];
  open: boolean;
  onClose: () => void;
  onSelect: (command: string) => void;
}

/** Simple fuzzy match: all characters of needle appear in order in haystack */
function fuzzyMatch(needle: string, haystack: string): boolean {
  const lower = haystack.toLowerCase();
  let j = 0;
  for (let i = 0; i < needle.length; i++) {
    const idx = lower.indexOf(needle[i], j);
    if (idx === -1) return false;
    j = idx + 1;
  }
  return true;
}

export default function SnippetPicker({
  snippets,
  open,
  onClose,
  onSelect,
}: SnippetPickerProps) {
  const [filter, setFilter] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const filtered = useMemo(() => {
    if (!filter) return snippets;
    const needle = filter.toLowerCase();
    return snippets.filter(
      (s) => fuzzyMatch(needle, s.name) || fuzzyMatch(needle, s.command)
    );
  }, [snippets, filter]);

  // Auto-focus and reset on open
  useEffect(() => {
    if (open) {
      setFilter("");
      setSelectedIndex(0);
      const t = setTimeout(() => inputRef.current?.focus(), 50);
      return () => clearTimeout(t);
    }
  }, [open]);

  // Reset selection when filter changes
  useEffect(() => {
    setSelectedIndex(0);
  }, [filter]);

  // Scroll selected item into view
  useEffect(() => {
    if (!listRef.current) return;
    const items = listRef.current.children;
    if (items[selectedIndex]) {
      (items[selectedIndex] as HTMLElement).scrollIntoView({ block: "nearest" });
    }
  }, [selectedIndex]);

  const handleSelect = useCallback(
    (command: string) => {
      onSelect(command);
      onClose();
    },
    [onSelect, onClose]
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      switch (e.key) {
        case "Escape":
          e.preventDefault();
          onClose();
          break;
        case "ArrowDown":
          e.preventDefault();
          setSelectedIndex((prev) => Math.min(prev + 1, filtered.length - 1));
          break;
        case "ArrowUp":
          e.preventDefault();
          setSelectedIndex((prev) => Math.max(prev - 1, 0));
          break;
        case "Enter":
          e.preventDefault();
          if (filtered[selectedIndex]) {
            handleSelect(filtered[selectedIndex].command);
          }
          break;
      }
    },
    [filtered, selectedIndex, handleSelect, onClose]
  );

  if (!open) return null;

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 50,
        display: "flex",
        alignItems: "flex-start",
        justifyContent: "center",
        paddingTop: "15vh",
        backgroundColor: "rgba(0, 0, 0, 0.5)",
      }}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        style={{
          width: "100%",
          maxWidth: 560,
          backgroundColor: "#1e1e1e",
          border: "1px solid #444",
          borderRadius: 8,
          boxShadow: "0 16px 48px rgba(0, 0, 0, 0.6)",
          overflow: "hidden",
          animation: "snippetPickerFadeIn 0.12s ease-out",
        }}
      >
        <style>{`
          @keyframes snippetPickerFadeIn {
            from { transform: translateY(-8px); opacity: 0; }
            to { transform: translateY(0); opacity: 1; }
          }
        `}</style>

        {/* Search input */}
        <div style={{ padding: "12px 12px 8px" }}>
          <input
            ref={inputRef}
            type="text"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type to search snippets..."
            style={{
              width: "100%",
              padding: "8px 12px",
              fontSize: 14,
              fontFamily: "'JetBrains Mono', monospace",
              backgroundColor: "#2a2a2a",
              color: "#e0e0e0",
              border: "1px solid #555",
              borderRadius: 6,
              outline: "none",
              boxSizing: "border-box",
            }}
          />
        </div>

        {/* Snippet list */}
        <div
          ref={listRef}
          style={{
            maxHeight: 400,
            overflowY: "auto",
            padding: "0 4px 8px",
          }}
        >
          {filtered.length === 0 && (
            <div
              style={{
                padding: "16px 12px",
                color: "#666",
                fontSize: 13,
                textAlign: "center",
              }}
            >
              No matching snippets
            </div>
          )}

          {filtered.map((snippet, idx) => (
            <div
              key={snippet.id}
              onClick={() => handleSelect(snippet.command)}
              onMouseEnter={() => setSelectedIndex(idx)}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 10,
                padding: "8px 12px",
                margin: "0 4px",
                borderRadius: 4,
                cursor: "pointer",
                backgroundColor:
                  idx === selectedIndex ? "#2a4a7a" : "transparent",
                transition: "background-color 0.1s",
              }}
            >
              {/* Name + command */}
              <div style={{ flex: 1, minWidth: 0 }}>
                <div
                  style={{
                    fontSize: 13,
                    fontWeight: 500,
                    color: "#e0e0e0",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  }}
                >
                  {snippet.name}
                </div>
                <div
                  style={{
                    fontSize: 12,
                    fontFamily: "'JetBrains Mono', monospace",
                    color: "#888",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                    marginTop: 2,
                  }}
                >
                  {snippet.command}
                </div>
              </div>

              {/* Scope badge */}
              <span
                style={{
                  fontSize: 10,
                  fontWeight: 600,
                  textTransform: "uppercase",
                  letterSpacing: "0.5px",
                  padding: "2px 6px",
                  borderRadius: 4,
                  backgroundColor:
                    snippet.scope === "global" ? "#1a3a2a" : "#2a2a4a",
                  color:
                    snippet.scope === "global" ? "#51cf66" : "#74c7ec",
                  flexShrink: 0,
                }}
              >
                {snippet.scope === "global" ? "global" : snippet.scope}
              </span>

              {/* Shortcut hint */}
              {snippet.shortcut && (
                <span
                  style={{
                    fontSize: 11,
                    fontFamily: "'JetBrains Mono', monospace",
                    color: "#666",
                    flexShrink: 0,
                  }}
                >
                  {snippet.shortcut}
                </span>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

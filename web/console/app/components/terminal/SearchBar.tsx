"use client";

import { useState, useEffect, useRef, useCallback, type RefObject } from "react";
import type { XTerminalHandle } from "../XTerminal";

interface SearchBarProps {
  termRef: RefObject<XTerminalHandle | null>;
  open: boolean;
  onClose: () => void;
}

export default function SearchBar({ termRef, open, onClose }: SearchBarProps) {
  const [query, setQuery] = useState("");
  const [useRegex, setUseRegex] = useState(false);
  const [caseSensitive, setCaseSensitive] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  // Auto-focus input when opened.
  useEffect(() => {
    if (!open) return undefined;
    const timeoutID = setTimeout(() => inputRef.current?.focus(), 50);
    return () => clearTimeout(timeoutID);
  }, [open]);

  // Clear state when closed.
  useEffect(() => {
    if (!open) {
      setQuery("");
    }
  }, [open]);

  const doSearchNext = useCallback(() => {
    if (!query) return;
    termRef.current?.searchNext(query, { regex: useRegex, caseSensitive });
  }, [termRef, query, useRegex, caseSensitive]);

  const doSearchPrevious = useCallback(() => {
    if (!query) return;
    termRef.current?.searchPrevious(query, { regex: useRegex, caseSensitive });
  }, [termRef, query, useRegex, caseSensitive]);

  const handleClose = useCallback(() => {
    termRef.current?.clearSearch();
    onClose();
  }, [termRef, onClose]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        handleClose();
      } else if (e.key === "Enter") {
        e.preventDefault();
        if (e.shiftKey) {
          doSearchPrevious();
        } else {
          doSearchNext();
        }
      }
    },
    [handleClose, doSearchNext, doSearchPrevious],
  );

  // Trigger search on query change.
  useEffect(() => {
    if (open && query) {
      termRef.current?.searchNext(query, { regex: useRegex, caseSensitive });
    } else if (open && !query) {
      termRef.current?.clearSearch();
    }
  }, [open, query, useRegex, caseSensitive, termRef]);

  if (!open) return null;

  const toggleButtonClass = (active: boolean) =>
    `rounded-md border px-2 py-0.5 font-mono text-xs transition-colors ${
      active
        ? "border-[var(--accent)] bg-[var(--accent)] text-[var(--accent-contrast)]"
        : "border-[var(--line)] bg-[var(--surface)] text-[var(--text)] hover:bg-[var(--hover)]"
    }`;

  return (
    <div
      className="absolute left-0 right-0 top-0 z-10 flex items-center gap-1.5 border-b border-[var(--line)] bg-[var(--panel-glass)] px-2 py-1.5 backdrop-blur-[12px]"
      style={{ animation: "searchBarSlideIn 0.15s ease-out" }}
    >
      <style>{`
        @keyframes searchBarSlideIn {
          from { transform: translateY(-100%); opacity: 0; }
          to { transform: translateY(0); opacity: 1; }
        }
      `}</style>

      <input
        ref={inputRef}
        type="text"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Search terminal..."
        className="min-w-0 flex-1 rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-1 font-mono text-xs text-[var(--text)] outline-none transition-colors focus:border-[var(--accent)]"
      />

      <button
        type="button"
        onClick={() => setUseRegex((v) => !v)}
        title="Use regular expression"
        className={toggleButtonClass(useRegex)}
      >
        .*
      </button>

      <button
        type="button"
        onClick={() => setCaseSensitive((v) => !v)}
        title="Match case"
        className={toggleButtonClass(caseSensitive)}
      >
        Aa
      </button>

      <button
        type="button"
        onClick={doSearchPrevious}
        title="Previous match (Shift+Enter)"
        className="rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-0.5 text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
      >
        {"\u2191"}
      </button>

      <button
        type="button"
        onClick={doSearchNext}
        title="Next match (Enter)"
        className="rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-0.5 text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
      >
        {"\u2193"}
      </button>

      <button
        type="button"
        onClick={handleClose}
        title="Close search (Escape)"
        className="rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-0.5 text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
      >
        {"\u2715"}
      </button>
    </div>
  );
}

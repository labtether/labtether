"use client";

import { useState, useEffect, useRef, useCallback, type RefObject } from "react";
import { ChevronDown, ChevronUp, X } from "lucide-react";
import { useTranslations } from "next-intl";
import type { XTerminalHandle } from "../XTerminal";

interface SearchBarProps {
  termRef: RefObject<XTerminalHandle | null>;
  open: boolean;
  onClose: () => void;
}

/** Find in the scrollback: Enter and Shift+Enter move between matches, Esc closes. */
export default function SearchBar({ termRef, open, onClose }: SearchBarProps) {
  const t = useTranslations("terminal.find");
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

  return (
    <div className="tm-find" role="search" aria-label={t("label")}>
      <input
        ref={inputRef}
        type="text"
        className="tm-find-q"
        aria-label={t("label")}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={t("placeholder")}
        spellCheck={false}
      />
      <button type="button" className="tm-find-b tm-find-t" aria-pressed={caseSensitive} onClick={() => setCaseSensitive((v) => !v)} title={t("matchCase")}>
        Aa
      </button>
      <button type="button" className="tm-find-b tm-find-t" aria-pressed={useRegex} onClick={() => setUseRegex((v) => !v)} title={t("regex")}>
        .*
      </button>
      <button type="button" className="tm-find-b" onClick={doSearchPrevious} disabled={!query} title={t("previous")} aria-label={t("previous")}>
        <ChevronUp aria-hidden="true" />
      </button>
      <button type="button" className="tm-find-b" onClick={doSearchNext} disabled={!query} title={t("next")} aria-label={t("next")}>
        <ChevronDown aria-hidden="true" />
      </button>
      <button type="button" className="tm-find-b" onClick={handleClose} title={t("close")} aria-label={t("close")}>
        <X aria-hidden="true" />
      </button>
    </div>
  );
}

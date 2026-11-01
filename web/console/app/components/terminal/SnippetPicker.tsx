"use client";

import { Search } from "lucide-react";
import { useTranslations } from "next-intl";
import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import type { TerminalSnippet } from "../../hooks/useTerminalSnippets";
import { Modal } from "../ui/Modal";

interface SnippetPickerProps {
  snippets: TerminalSnippet[];
  open: boolean;
  onClose: () => void;
  onSelect: (command: string) => void;
}

/** Every character of the needle appears in order in the haystack. */
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

/** Saved commands: type to filter, ↑↓ to move, Enter types the command into the focused pane. */
export default function SnippetPicker({ snippets, open, onClose, onSelect }: SnippetPickerProps) {
  const t = useTranslations("terminal.snippets");
  const [filter, setFilter] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const filtered = useMemo(() => {
    if (!filter) return snippets;
    const needle = filter.toLowerCase();
    return snippets.filter((s) => fuzzyMatch(needle, s.name) || fuzzyMatch(needle, s.command));
  }, [snippets, filter]);

  useEffect(() => {
    if (!open) return undefined;
    setFilter("");
    setSelectedIndex(0);
    const timer = setTimeout(() => inputRef.current?.focus(), 30);
    return () => clearTimeout(timer);
  }, [open]);

  useEffect(() => setSelectedIndex(0), [filter]);

  useEffect(() => {
    const item = listRef.current?.children[selectedIndex] as HTMLElement | undefined;
    item?.scrollIntoView({ block: "nearest" });
  }, [selectedIndex]);

  const handleSelect = useCallback(
    (command: string) => {
      onSelect(command);
      onClose();
    },
    [onSelect, onClose],
  );

  const handleKeyDown = (event: KeyboardEvent) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setSelectedIndex((prev) => Math.min(prev + 1, filtered.length - 1));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setSelectedIndex((prev) => Math.max(prev - 1, 0));
    } else if (event.key === "Enter") {
      event.preventDefault();
      const snippet = filtered[selectedIndex];
      if (snippet) handleSelect(snippet.command);
    }
  };

  return (
    <Modal open={open} onClose={onClose} ariaTitle={t("title")} className="tm-snip">
      <div className="tm-snip-in">
        <Search aria-hidden="true" />
        <input
          ref={inputRef}
          type="text"
          role="combobox"
          aria-expanded="true"
          aria-controls="tm-snip-list"
          aria-label={t("search")}
          value={filter}
          placeholder={t("search")}
          onChange={(event) => setFilter(event.target.value)}
          onKeyDown={handleKeyDown}
        />
      </div>
      <div ref={listRef} id="tm-snip-list" className="tm-snip-list" role="listbox" aria-label={t("title")}>
        {filtered.length === 0 ? (
          <p className="tm-none">{snippets.length === 0 ? t("none") : t("noMatch")}</p>
        ) : (
          filtered.map((snippet, index) => (
            <button
              key={snippet.id}
              type="button"
              role="option"
              aria-selected={index === selectedIndex}
              className="tm-snip-item"
              onClick={() => handleSelect(snippet.command)}
              onMouseEnter={() => setSelectedIndex(index)}
            >
              <span className="tm-snip-t">
                <span className="tm-nm">{snippet.name}</span>
                <span className="tm-snip-cmd">{snippet.command}</span>
              </span>
              <span className="tm-snip-scope">{snippet.scope === "global" ? t("everywhere") : snippet.scope}</span>
              {snippet.shortcut ? <kbd>{snippet.shortcut}</kbd> : null}
            </button>
          ))
        )}
      </div>
    </Modal>
  );
}

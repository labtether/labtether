"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Command } from "cmdk";
import { Search } from "lucide-react";
import { usePalette, type PaletteItem } from "../contexts/PaletteContext";

const MAX_PER_GROUP = 5;

interface GroupedResults {
  group: string;
  items: PaletteItem[];
  hasMore: boolean;
}

export function CommandPalette() {
  const { open, setOpen, getProviders } = usePalette();
  const [query, setQuery] = useState("");
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());
  const inputRef = useRef<HTMLInputElement>(null);

  // Global keyboard shortcut: Cmd+K / Ctrl+K to toggle
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        setOpen((prev) => {
          if (prev) return false;
          setQuery("");
          setExpandedGroups(new Set());
          return true;
        });
      }
      if (e.key === "Escape") {
        setOpen(false);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [setOpen]);

  // Focus input when palette opens
  useEffect(() => {
    if (open) {
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  // Compute grouped results from providers
  const groupedResults = useMemo((): GroupedResults[] => {
    const providers = getProviders();
    const trimmed = query.trim();
    const prefixChar = trimmed.length > 0 ? trimmed[0] : "";
    const hasPrefix = prefixChar === ">" || prefixChar === "!";

    const groups: GroupedResults[] = [];

    for (const provider of providers) {
      if (hasPrefix && provider.shortcut !== prefixChar) continue;
      if (!hasPrefix && provider.shortcut && trimmed.length > 0) continue;

      try {
        const allItems = provider.search(query);
        if (allItems.length === 0) continue;

        const isExpanded = expandedGroups.has(provider.group);
        const items = isExpanded ? allItems : allItems.slice(0, MAX_PER_GROUP);
        const hasMore = allItems.length > MAX_PER_GROUP && !isExpanded;

        groups.push({ group: provider.group, items, hasMore });
      } catch {
        // Silently skip providers that throw
      }
    }

    return groups;
  }, [query, getProviders, expandedGroups]);

  const handleSelect = useCallback(
    (item: PaletteItem) => {
      setOpen(false);
      setQuery("");
      item.action();
    },
    [setOpen],
  );

  const handleClose = useCallback(() => {
    setOpen(false);
    setQuery("");
  }, [setOpen]);

  const toggleGroupExpand = useCallback((group: string) => {
    setExpandedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(group)) {
        next.delete(group);
      } else {
        next.add(group);
      }
      return next;
    });
  }, []);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-[20vh] bg-black/50 backdrop-blur-sm"
      onClick={handleClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        className="w-full max-w-lg bg-[var(--panel)] border border-[var(--line)] rounded-lg shadow-2xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
        onKeyDown={(e) => { if (e.key === "Escape") handleClose(); }}
      >
        <Command
          label="Command palette"
          shouldFilter={false}
          loop
        >
          {/* Search input */}
          <div className="flex items-center gap-2.5 px-4 h-12 border-b border-[var(--line)]">
            <Search size={16} strokeWidth={1.5} className="text-[var(--muted)]" />
            <Command.Input
              ref={inputRef}
              className="flex-1 bg-transparent text-sm text-[var(--text)] placeholder:text-[var(--muted)] outline-none"
              placeholder="Type a command or search..."
              value={query}
              onValueChange={setQuery}
            />
            <kbd className="px-1.5 py-0.5 rounded text-[10px] text-[var(--muted)] border border-[var(--line)]">
              ESC
            </kbd>
          </div>

          {/* Results */}
          <Command.List className="max-h-80 overflow-y-auto py-1">
            <Command.Empty className="px-4 py-6 text-center text-sm text-[var(--muted)]">
              No results found
            </Command.Empty>

            {groupedResults.map(({ group, items, hasMore }) => (
              <Command.Group
                key={group}
                heading={group}
                className="[&_[cmdk-group-heading]]:px-4 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:font-mono [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-wider [&_[cmdk-group-heading]]:text-[var(--muted)]"
              >
                {items.map((item) => {
                  const Icon = item.icon;
                  const isCode = group === "Snippets" || group === "Quick Connect";

                  return (
                    <Command.Item
                      key={item.id}
                      value={item.id}
                      onSelect={() => handleSelect(item)}
                      className="flex items-center gap-3 px-4 py-2 text-sm cursor-pointer transition-colors duration-150 text-[var(--muted)] aria-selected:bg-[var(--surface)] aria-selected:text-[var(--text)] hover:bg-[var(--hover)]"
                    >
                      {Icon && <Icon size={16} strokeWidth={1.5} className="shrink-0 opacity-60" />}
                      <span className={`flex-1 truncate ${isCode ? "font-mono" : ""}`}>{item.label}</span>
                      {item.description && (
                        <span className={`text-xs text-[var(--muted)] truncate ${isCode ? "font-mono" : ""}`}>
                          {item.description}
                        </span>
                      )}
                    </Command.Item>
                  );
                })}
                {hasMore && (
                  <Command.Item
                    value={`__show-more-${group}`}
                    onSelect={() => toggleGroupExpand(group)}
                    className="w-full px-4 py-1.5 text-[10px] text-[var(--accent)] aria-selected:text-[var(--text)] text-left font-mono uppercase tracking-wider transition-colors cursor-pointer"
                  >
                    Show all
                  </Command.Item>
                )}
              </Command.Group>
            ))}
          </Command.List>
        </Command>
      </div>
    </div>
  );
}

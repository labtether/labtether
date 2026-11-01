"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Command } from "cmdk";
import * as Dialog from "@radix-ui/react-dialog";
import { useTranslations } from "next-intl";
import { usePalette, type PaletteItem } from "../contexts/PaletteContext";

const MAX_PER_GROUP = 5;
const CODE_GROUPS = new Set(["Snippets", "Quick connect"]);

interface GroupedResults {
  group: string;
  items: PaletteItem[];
  hasMore: boolean;
}

/**
 * 2.0 ⌘K (kit.css .kt-pal): a scrim, a 56 px search line, then grouped results in 36 px rows with a quiet hint on the
 * right. ⌘K / Ctrl+K toggles it, Escape closes it, focus stays inside while it is open.
 */
export function CommandPalette() {
  const t = useTranslations("common");
  const { open, setOpen, getProviders } = usePalette();
  const [query, setQuery] = useState("");
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());

  const reset = useCallback(() => {
    setQuery("");
    setExpandedGroups(new Set());
  }, []);

  // Global keyboard shortcut: Cmd+K / Ctrl+K to toggle.
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        reset();
        setOpen((prev) => !prev);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [setOpen, reset]);

  // Compute grouped results from providers.
  const groupedResults = useMemo((): GroupedResults[] => {
    if (!open) return [];
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
        // A provider that throws is skipped; the others still answer.
      }
    }

    return groups;
  }, [open, query, getProviders, expandedGroups]);

  const handleSelect = useCallback(
    (item: PaletteItem) => {
      setOpen(false);
      setQuery("");
      item.action();
    },
    [setOpen],
  );

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

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) reset();
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="kt-scrim pal">
          <Dialog.Content className="kt-pal" aria-describedby={undefined}>
            <Dialog.Title className="sr-only">{t("search.label")}</Dialog.Title>
            <Command label={t("search.label")} shouldFilter={false} loop>
              <div className="kt-pal-in">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
                  <circle cx="11" cy="11" r="7" />
                  <path d="m20 20-3.5-3.5" />
                </svg>
                <Command.Input
                  className="kt-pal-q"
                  placeholder={t("palette.placeholder")}
                  value={query}
                  onValueChange={setQuery}
                />
              </div>

              <Command.List className="kt-pal-list">
                <Command.Empty className="kt-pal-empty">{t("noResults")}</Command.Empty>

                {groupedResults.map(({ group, items, hasMore }) => {
                  const isCode = CODE_GROUPS.has(group);
                  return (
                    <Command.Group key={group} heading={group}>
                      {items.map((item) => (
                        <Command.Item key={item.id} value={item.id} onSelect={() => handleSelect(item)} className="kt-pal-item">
                          <span className={`kt-pal-lbl${isCode ? " kt-mono" : ""}`}>{item.label}</span>
                          {item.description ? <span className="kt-pal-hint">{item.description}</span> : null}
                        </Command.Item>
                      ))}
                      {hasMore ? (
                        <Command.Item
                          value={`__show-more-${group}`}
                          onSelect={() => toggleGroupExpand(group)}
                          className="kt-pal-item kt-pal-more"
                        >
                          {t("palette.showAll")}
                        </Command.Item>
                      ) : null}
                    </Command.Group>
                  );
                })}
              </Command.List>
            </Command>
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

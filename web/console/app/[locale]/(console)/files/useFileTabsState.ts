"use client";

import { useCallback, useMemo, useState } from "react";

// Files tabs. Each tab browses one device (through its agent) or one saved connection, or is a new tab that shows
// the source picker. A tab may show a second pane with another source (or the same one) beside it.

export type TabType = "new" | "agent" | "connection";

export type SplitTarget = { type: "agent" | "connection"; sourceId: string; protocol: string; label: string };

export interface FileTab {
  id: string;
  type: TabType;
  label: string;
  protocol?: string;    // "agent" | "sftp" | "smb" | "ftp" | "webdav"
  sourceId?: string;    // asset ID or connection ID
  split?: SplitTarget | null;
}

export type FileSource =
  | { type: "agent"; assetId: string }
  | { type: "connection"; connectionId: string };

function makeTab(partial: Partial<FileTab>): FileTab {
  return {
    id: crypto.randomUUID(),
    type: partial.type ?? "new",
    label: partial.label ?? "",
    protocol: partial.protocol,
    sourceId: partial.sourceId,
    split: partial.split ?? null,
  };
}

export function sourceOf(tab: { type: TabType; sourceId?: string } | null | undefined): FileSource | null {
  if (!tab?.sourceId) return null;
  if (tab.type === "agent") return { type: "agent", assetId: tab.sourceId };
  if (tab.type === "connection") return { type: "connection", connectionId: tab.sourceId };
  return null;
}

/** "a:<asset>" or "c:<connection>": which source a clipboard or transfer belongs to. */
export function sourceKey(source: FileSource | null): string {
  if (!source) return "";
  return source.type === "agent" ? `a:${source.assetId}` : `c:${source.connectionId}`;
}

/** The source a key names, or null. */
export function sourceFromKey(key: string): FileSource | null {
  if (key.startsWith("a:")) return { type: "agent", assetId: key.slice(2) };
  if (key.startsWith("c:")) return { type: "connection", connectionId: key.slice(2) };
  return null;
}

export function useFileTabsState() {
  const [tabs, setTabs] = useState<FileTab[]>([]);
  const [activeTabId, setActiveTabId] = useState<string>("");

  const activeTab = useMemo(
    () => tabs.find((t) => t.id === activeTabId) ?? tabs[0] ?? null,
    [tabs, activeTabId],
  );
  const activeSource = useMemo(() => sourceOf(activeTab), [activeTab]);
  const splitSource = useMemo(() => sourceOf(activeTab?.split ?? null), [activeTab]);

  const addTab = useCallback((partial: Partial<FileTab>) => {
    const tab = makeTab(partial);
    setTabs((prev) => [...prev, tab]);
    setActiveTabId(tab.id);
  }, []);

  /** A source picked on the picker: it fills the new tab the picker shows in, or opens a tab when there is none. */
  const openInTab = useCallback((partial: Partial<FileTab>) => {
    const current = activeTab;
    if (current?.type === "new") {
      setTabs((prev) => prev.map((t) => (t.id === current.id ? { ...makeTab(partial), id: t.id } : t)));
      setActiveTabId(current.id);
      return;
    }
    addTab(partial);
  }, [activeTab, addTab]);

  const removeTab = useCallback((tabId: string) => {
    const idx = tabs.findIndex((t) => t.id === tabId);
    if (idx === -1) return;
    const next = tabs.filter((t) => t.id !== tabId);
    setTabs(next);
    setActiveTabId((cur) => (cur === tabId ? next[Math.min(idx, next.length - 1)]?.id ?? "" : cur));
  }, [tabs]);

  const updateTab = useCallback((tabId: string, updates: Partial<FileTab>) => {
    setTabs((prev) => prev.map((t) => (t.id === tabId ? { ...t, ...updates, id: t.id } : t)));
  }, []);

  /** Open (or replace) the active tab's second pane; null closes it. */
  const setSplit = useCallback((target: SplitTarget | null) => {
    if (!activeTab) return;
    updateTab(activeTab.id, { split: target });
  }, [activeTab, updateTab]);

  return {
    tabs,
    activeTabId: activeTab?.id ?? "",
    activeTab,
    activeSource,
    splitSource,
    splitMode: Boolean(activeTab?.split && activeTab.type !== "new"),
    addTab,
    openInTab,
    removeTab,
    setActiveTab: setActiveTabId,
    setSplit,
    updateTab,
  };
}

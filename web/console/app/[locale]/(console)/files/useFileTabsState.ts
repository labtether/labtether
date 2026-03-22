"use client";

import { useCallback, useMemo, useState } from "react";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type TabType = "new" | "agent" | "connection";

export interface FileTab {
  id: string;
  type: TabType;
  label: string;
  protocol?: string;    // "agent" | "sftp" | "smb" | "ftp" | "webdav"
  sourceId?: string;    // asset ID or connection ID
  splitWith?: string;   // ID of the tab shown in the right split pane (null if not split)
}

export type FileSource =
  | { type: "agent"; assetId: string }
  | { type: "connection"; connectionId: string };

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeNewTab(): FileTab {
  return {
    id: crypto.randomUUID(),
    type: "new",
    label: "New Tab",
  };
}

function deriveSource(tab: FileTab): FileSource | null {
  if (tab.type === "agent" && tab.sourceId) {
    return { type: "agent", assetId: tab.sourceId };
  }
  if (tab.type === "connection" && tab.sourceId) {
    return { type: "connection", connectionId: tab.sourceId };
  }
  return null;
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useFileTabsState() {
  const [tabs, setTabs] = useState<FileTab[]>(() => [makeNewTab()]);
  const [activeTabId, setActiveTabId] = useState<string>(() => tabs[0].id);
  const [splitMode, setSplitMode] = useState(false);

  const activeTab = useMemo(
    () => tabs.find((t) => t.id === activeTabId) ?? tabs[0],
    [tabs, activeTabId],
  );

  const activeSource = useMemo<FileSource | null>(
    () => (activeTab ? deriveSource(activeTab) : null),
    [activeTab],
  );

  const splitSource = useMemo<FileSource | null>(() => {
    if (!splitMode || !activeTab?.splitWith) return null;
    const splitTab = tabs.find((t) => t.id === activeTab.splitWith);
    return splitTab ? deriveSource(splitTab) : null;
  }, [splitMode, activeTab, tabs]);

  const addTab = useCallback((partial: Partial<FileTab>) => {
    const tab: FileTab = {
      id: crypto.randomUUID(),
      type: partial.type ?? "new",
      label: partial.label ?? "New Tab",
      protocol: partial.protocol,
      sourceId: partial.sourceId,
      splitWith: partial.splitWith,
    };
    setTabs((prev) => [...prev, tab]);
    setActiveTabId(tab.id);
  }, []);

  const removeTab = useCallback((tabId: string) => {
    // React calls updater functions synchronously, so newActiveId is set
    // before the setActiveTabId call below — no queueMicrotask needed.
    let newActiveId: string | undefined;
    setTabs((prev) => {
      const idx = prev.findIndex((t) => t.id === tabId);
      if (idx === -1) return prev;

      let next = prev
        .filter((t) => t.id !== tabId)
        .map((t) => (t.splitWith === tabId ? { ...t, splitWith: undefined } : t));

      if (next.length === 0) {
        next = [makeNewTab()];
      }

      newActiveId = next[Math.min(idx, next.length - 1)].id;
      return next;
    });
    if (newActiveId) {
      setActiveTabId((cur) => (cur === tabId ? newActiveId! : cur));
    }
  }, []);

  const updateTab = useCallback((tabId: string, updates: Partial<FileTab>) => {
    setTabs((prev) =>
      prev.map((t) => (t.id === tabId ? { ...t, ...updates, id: t.id } : t)),
    );
  }, []);

  const toggleSplit = useCallback(() => {
    setSplitMode((prev) => !prev);
  }, []);

  const setSplitTarget = useCallback(
    (sourceId: string, protocol: string) => {
      if (!activeTab) return;
      const isAgent = protocol === "agent";
      const splitTab: FileTab = {
        id: crypto.randomUUID(),
        type: isAgent ? "agent" : "connection",
        label: `${protocol.toUpperCase()} split`,
        protocol,
        sourceId,
      };
      setTabs((prev) => [...prev, splitTab]);
      updateTab(activeTab.id, { splitWith: splitTab.id });
      setSplitMode(true);
    },
    [activeTab, updateTab],
  );

  return {
    tabs,
    activeTabId,
    activeTab,
    splitMode,
    activeSource,
    splitSource,
    addTab,
    removeTab,
    setActiveTab: setActiveTabId,
    toggleSplit,
    setSplitTarget,
    updateTab,
  };
}

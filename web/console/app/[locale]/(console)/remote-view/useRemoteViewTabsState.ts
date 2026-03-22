"use client";

import { useState, useCallback, useMemo } from "react";
import type { RemoteViewTab, RemoteViewConnectionState } from "./types";

function newTab(): RemoteViewTab {
  return {
    id: crypto.randomUUID(),
    type: "new",
    label: "New Tab",
    connectionState: "idle",
  };
}

export function useRemoteViewTabsState() {
  const [tabs, setTabs] = useState<RemoteViewTab[]>(() => [newTab()]);
  const [activeTabId, setActiveTabId] = useState<string>(() => tabs[0].id);

  const activeTab = useMemo(
    () => tabs.find((t) => t.id === activeTabId) ?? tabs[0],
    [tabs, activeTabId],
  );

  const addTab = useCallback((partial?: Partial<RemoteViewTab>) => {
    const tab: RemoteViewTab = { ...newTab(), ...partial };
    setTabs((prev) => [...prev, tab]);
    setActiveTabId(tab.id);
  }, []);

  const removeTab = useCallback(
    (tabId: string) => {
      setTabs((prev) => {
        const next = prev.filter((t) => t.id !== tabId);
        if (next.length === 0) {
          const fallback = newTab();
          setActiveTabId(fallback.id);
          return [fallback];
        }
        if (activeTabId === tabId) {
          const idx = prev.findIndex((t) => t.id === tabId);
          const target = next[Math.min(idx, next.length - 1)];
          setActiveTabId(target.id);
        }
        return next;
      });
    },
    [activeTabId],
  );

  const updateTab = useCallback(
    (tabId: string, updates: Partial<RemoteViewTab>) => {
      setTabs((prev) =>
        prev.map((t) => (t.id === tabId ? { ...t, ...updates } : t)),
      );
    },
    [],
  );

  const setConnectionState = useCallback(
    (tabId: string, state: RemoteViewConnectionState) => {
      updateTab(tabId, {
        connectionState: state,
        ...(state === "connected" ? { lastConnectedAt: Date.now() } : {}),
      });
    },
    [updateTab],
  );

  return {
    tabs,
    activeTabId,
    activeTab,
    addTab,
    removeTab,
    setActiveTab: setActiveTabId,
    updateTab,
    setConnectionState,
  };
}

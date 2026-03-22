"use client";

import { useState, useEffect, useCallback, useRef, useMemo } from "react";

export interface WorkspacePane {
  targetNodeId: string;
  tmuxSession?: string;
}

export interface WorkspaceTab {
  id: string;
  name: string;
  layout: string;
  panes: WorkspacePane[];
  panel_sizes: PanelSizes;
  sort_order: number;
}

// Panel sizes for single-axis layouts (columns, rows, main-side, main-bottom)
type SingleAxisSizes = number[];

// Panel sizes for grid layout (nested groups)
interface GridSizes {
  outer: number[];
  top: number[];
  bottom: number[];
}

// null means "use default ratios for the layout mode"
export type PanelSizes = SingleAxisSizes | GridSizes | null;

function asObject(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function asString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function asFiniteNumber(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function normalizePanelSizes(value: unknown): PanelSizes {
  if (value == null) return null;
  if (Array.isArray(value)) {
    const nums = value.filter(
      (v): v is number => typeof v === "number" && Number.isFinite(v)
    );
    return nums.length > 0 ? nums : null;
  }
  if (typeof value === "object" && value !== null) {
    const obj = value as Record<string, unknown>;
    if (
      Array.isArray(obj.outer) &&
      Array.isArray(obj.top) &&
      Array.isArray(obj.bottom)
    ) {
      const outer = (obj.outer as unknown[]).filter(
        (v): v is number => typeof v === "number" && Number.isFinite(v)
      );
      const top = (obj.top as unknown[]).filter(
        (v): v is number => typeof v === "number" && Number.isFinite(v)
      );
      const bottom = (obj.bottom as unknown[]).filter(
        (v): v is number => typeof v === "number" && Number.isFinite(v)
      );
      if (outer.length > 0 && top.length > 0 && bottom.length > 0) {
        return { outer, top, bottom };
      }
    }
  }
  return null;
}

function normalizeWorkspacePane(value: unknown): WorkspacePane | null {
  const raw = asObject(value);
  if (!raw) {
    return null;
  }
  return {
    targetNodeId: asString(raw.targetNodeId),
    tmuxSession: asString(raw.tmuxSession) || undefined,
  };
}

function normalizeWorkspaceTab(value: unknown): WorkspaceTab | null {
  const raw = asObject(value);
  if (!raw) {
    return null;
  }
  const id = asString(raw.id);
  if (id === "") {
    return null;
  }
  const name = asString(raw.name) || "Untitled";
  const panes = Array.isArray(raw.panes)
    ? raw.panes.map(normalizeWorkspacePane).filter((pane): pane is WorkspacePane => pane !== null)
    : [];
  return {
    id,
    name,
    layout: asString(raw.layout) || "single",
    panes,
    panel_sizes: normalizePanelSizes(raw.panel_sizes),
    sort_order: asFiniteNumber(raw.sort_order),
  };
}

function normalizeWorkspaceTabList(value: unknown): WorkspaceTab[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map(normalizeWorkspaceTab)
    .filter((tab): tab is WorkspaceTab => tab !== null);
}

function normalizeWorkspaceTabsPayload(value: unknown): WorkspaceTab[] {
  if (Array.isArray(value)) {
    return normalizeWorkspaceTabList(value);
  }
  const raw = asObject(value);
  return normalizeWorkspaceTabList(raw?.tabs);
}

function normalizeWorkspaceTabPayload(value: unknown): WorkspaceTab | null {
  const raw = asObject(value);
  if (raw && "tab" in raw) {
    return normalizeWorkspaceTab(raw.tab);
  }
  return normalizeWorkspaceTab(value);
}

export function useWorkspaceTabs() {
  const [tabs, setTabs] = useState<WorkspaceTab[]>([]);
  const [activeTabId, setActiveTabId] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const abortRef = useRef<AbortController | null>(null);
  const mutationAbortRef = useRef<AbortController | null>(null);
  const updateAbortControllers = useRef<Map<string, AbortController>>(new Map());
  const debounceTimers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
  const pendingUpdates = useRef<Map<string, Partial<Omit<WorkspaceTab, "id">>>>(new Map());
  const fetchSeqRef = useRef(0);
  const mountedRef = useRef(true);
  const tabsRef = useRef<WorkspaceTab[]>([]);

  useEffect(() => {
    tabsRef.current = tabs;
  }, [tabs]);

  const fetchTabs = useCallback(async () => {
    const fetchSeq = ++fetchSeqRef.current;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    try {
      const res = await fetch("/api/terminal/workspace/tabs", {
        cache: "no-store",
        signal: controller.signal,
      });
      if (res.ok) {
        const data = await res.json().catch(() => null);
        const fetched = normalizeWorkspaceTabsPayload(data);
        if (controller.signal.aborted || fetchSeq !== fetchSeqRef.current || !mountedRef.current) {
          return;
        }

        if (fetched.length === 0) {
          // Auto-create a default tab if none exist
          const createRes = await fetch("/api/terminal/workspace/tabs", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ name: "Default", layout: "single", panes: [] }),
            signal: controller.signal,
          });
          if (createRes.ok) {
            const created = await createRes.json().catch(() => null);
            const newTab = normalizeWorkspaceTabPayload(created);
            if (newTab?.id) {
              if (controller.signal.aborted || fetchSeq !== fetchSeqRef.current || !mountedRef.current) {
                return;
              }
              setTabs([newTab]);
              setActiveTabId(newTab.id);
              return;
            }
          }
        }

        if (controller.signal.aborted || fetchSeq !== fetchSeqRef.current || !mountedRef.current) {
          return;
        }
        setTabs(fetched);
        // Set active tab to first if not already set or if current is gone
        if (fetched.length > 0) {
          setActiveTabId((prev) => {
            if (prev && fetched.some((t) => t.id === prev)) return prev;
            return fetched[0].id;
          });
        } else {
          setActiveTabId("");
        }
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      // Keep existing tabs on error
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      if (fetchSeq === fetchSeqRef.current && mountedRef.current) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    void fetchTabs();
    const timers = debounceTimers.current;
    const pending = pendingUpdates.current;
    const updateControllers = updateAbortControllers.current;
    return () => {
      mountedRef.current = false;
      abortRef.current?.abort();
      abortRef.current = null;
      mutationAbortRef.current?.abort();
      mutationAbortRef.current = null;
      for (const controller of updateControllers.values()) {
        controller.abort();
      }
      updateControllers.clear();
      // Cleanup debounce timers on unmount
      for (const timer of timers.values()) {
        clearTimeout(timer);
      }
      timers.clear();
      pending.clear();
    };
  }, [fetchTabs]);

  const createTab = useCallback(
    async (name?: string) => {
      mutationAbortRef.current?.abort();
      const controller = new AbortController();
      mutationAbortRef.current = controller;

      const tabName = name ?? `Tab ${tabsRef.current.length + 1}`;
      const res = await fetch("/api/terminal/workspace/tabs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: tabName, layout: "single", panes: [] }),
        signal: controller.signal,
      });
      if (!res.ok) {
        if (mutationAbortRef.current === controller) {
          mutationAbortRef.current = null;
        }
        throw new Error(await readAPIError(res, "Failed to create tab"));
      }
      const created = await res.json().catch(() => null);
      const newTab = normalizeWorkspaceTabPayload(created);
      if (!newTab) {
        if (mutationAbortRef.current === controller) {
          mutationAbortRef.current = null;
        }
        throw new Error("Created tab response missing tab");
      }
      await fetchTabs();
      if (newTab.id && mountedRef.current) {
        setActiveTabId(newTab.id);
      }
      if (mutationAbortRef.current === controller) {
        mutationAbortRef.current = null;
      }
      return newTab;
    },
    [fetchTabs]
  );

  const updateTab = useCallback(
    async (id: string, data: Partial<Omit<WorkspaceTab, "id">>) => {
      // Optimistic local update
      setTabs((prev) =>
        prev.map((t) => (t.id === id ? { ...t, ...data } : t))
      );

      const mergedPending = {
        ...(pendingUpdates.current.get(id) ?? {}),
        ...data,
      };
      pendingUpdates.current.set(id, mergedPending);

      // Debounce the PUT request to avoid excessive updates for layout/pane changes
      const existing = debounceTimers.current.get(id);
      if (existing) clearTimeout(existing);
      const existingController = updateAbortControllers.current.get(id);
      if (existingController) {
        existingController.abort();
        updateAbortControllers.current.delete(id);
      }

      const timer = setTimeout(async () => {
        debounceTimers.current.delete(id);
        const payload = pendingUpdates.current.get(id);
        pendingUpdates.current.delete(id);
        if (!payload) return;
        const controller = new AbortController();
        updateAbortControllers.current.set(id, controller);
        try {
          const res = await fetch(
            `/api/terminal/workspace/tabs/${encodeURIComponent(id)}`,
            {
              method: "PUT",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify(payload),
              signal: controller.signal,
            }
          );
          if (!res.ok) {
            // Revert on error
            await fetchTabs();
          }
        } catch (error) {
          if (error instanceof DOMException && error.name === "AbortError") {
            return;
          }
          await fetchTabs();
        } finally {
          if (updateAbortControllers.current.get(id) === controller) {
            updateAbortControllers.current.delete(id);
          }
        }
      }, 500);

      debounceTimers.current.set(id, timer);
    },
    [fetchTabs]
  );

  const deleteTab = useCallback(
    async (id: string) => {
      const pendingTimer = debounceTimers.current.get(id);
      if (pendingTimer) {
        clearTimeout(pendingTimer);
        debounceTimers.current.delete(id);
      }
      pendingUpdates.current.delete(id);
      const existingController = updateAbortControllers.current.get(id);
      if (existingController) {
        existingController.abort();
        updateAbortControllers.current.delete(id);
      }

      mutationAbortRef.current?.abort();
      const controller = new AbortController();
      mutationAbortRef.current = controller;

      setTabs((prev) => prev.filter((tab) => tab.id !== id));
      setActiveTabId((prev) => (prev === id ? "" : prev));

      const res = await fetch(
        `/api/terminal/workspace/tabs/${encodeURIComponent(id)}`,
        { method: "DELETE", signal: controller.signal }
      );
      if (!res.ok) {
        if (mutationAbortRef.current === controller) {
          mutationAbortRef.current = null;
        }
        await fetchTabs();
        throw new Error(await readAPIError(res, "Failed to delete tab"));
      }
      await fetchTabs();
      if (mutationAbortRef.current === controller) {
        mutationAbortRef.current = null;
      }
    },
    [fetchTabs]
  );

  const setActiveTab = useCallback((id: string) => {
    setActiveTabId(id);
  }, []);

  const activeTab = useMemo(
    () => tabs.find((t) => t.id === activeTabId) ?? null,
    [tabs, activeTabId]
  );

  return {
    tabs,
    activeTab,
    activeTabId,
    setActiveTab,
    createTab,
    updateTab,
    deleteTab,
    loading,
  };
}

async function readAPIError(response: Response, fallback: string): Promise<string> {
  const contentType = response.headers.get("content-type") ?? "";
  if (contentType.includes("application/json")) {
    const payload = (await response.json().catch(() => null)) as { error?: unknown } | null;
    if (typeof payload?.error === "string" && payload.error.trim() !== "") {
      return payload.error.trim();
    }
    return fallback;
  }

  const text = (await response.text().catch(() => "")).trim();
  if (text !== "") {
    return text;
  }
  return fallback;
}

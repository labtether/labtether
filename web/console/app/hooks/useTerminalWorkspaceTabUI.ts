"use client";

import {
  useState,
  useRef,
  useCallback,
  useEffect,
  useMemo,
  type MouseEvent as ReactMouseEvent,
} from "react";
import type { WorkspaceTab } from "./useWorkspaceTabs";

export type TabContextMenuState = {
  tabId: string;
  x: number;
  y: number;
};

type CreateTabResult = WorkspaceTab | { id?: string } | null | undefined;

type UseTerminalWorkspaceTabUIOptions = {
  tabs: WorkspaceTab[];
  activeTabId: string;
  setActiveTab: (id: string) => void;
  createTab: (name?: string) => Promise<CreateTabResult>;
  updateTab: (id: string, data: Partial<Omit<WorkspaceTab, "id">>) => Promise<unknown>;
  deleteTab: (id: string) => Promise<unknown>;
};

function clampContextMenuPosition(
  x: number,
  y: number,
  bounds?: { width: number; height: number },
  width = 220,
  height = 260,
  padding = 8,
) {
  const maxWidth = bounds?.width ?? (typeof window !== "undefined" ? window.innerWidth : width + padding * 2);
  const maxHeight = bounds?.height ?? (typeof window !== "undefined" ? window.innerHeight : height + padding * 2);
  return {
    x: Math.max(padding, Math.min(x, maxWidth - width - padding)),
    y: Math.max(padding, Math.min(y, maxHeight - height - padding)),
  };
}

export function useTerminalWorkspaceTabUI({
  tabs,
  activeTabId,
  setActiveTab,
  createTab,
  updateTab,
  deleteTab,
}: UseTerminalWorkspaceTabUIOptions) {
  const [layoutMenuOpen, setLayoutMenuOpen] = useState(false);
  const [workspaceError, setWorkspaceError] = useState<string | null>(null);
  const [tabMenu, setTabMenu] = useState<TabContextMenuState | null>(null);
  const [editingTabId, setEditingTabId] = useState<string | null>(null);
  const [editingTabName, setEditingTabName] = useState("");

  const layoutMenuRef = useRef<HTMLDivElement | null>(null);
  const tabMenuHostRef = useRef<HTMLDivElement | null>(null);
  const tabRenameInputRef = useRef<HTMLInputElement | null>(null);
  const skipRenameCommitRef = useRef(false);

  useEffect(() => {
    if (!layoutMenuOpen) return undefined;
    const onPointerDown = (event: MouseEvent) => {
      if (!layoutMenuRef.current) return;
      if (layoutMenuRef.current.contains(event.target as Node)) return;
      setLayoutMenuOpen(false);
    };
    window.addEventListener("pointerdown", onPointerDown);
    return () => window.removeEventListener("pointerdown", onPointerDown);
  }, [layoutMenuOpen]);

  useEffect(() => {
    if (!workspaceError) return undefined;
    const timer = window.setTimeout(() => setWorkspaceError(null), 6000);
    return () => window.clearTimeout(timer);
  }, [workspaceError]);

  useEffect(() => {
    if (!tabMenu) return undefined;
    const onEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setTabMenu(null);
      }
    };
    window.addEventListener("keydown", onEscape);
    return () => window.removeEventListener("keydown", onEscape);
  }, [tabMenu]);

  useEffect(() => {
    if (!editingTabId) return undefined;
    const timer = window.setTimeout(() => {
      tabRenameInputRef.current?.focus();
      tabRenameInputRef.current?.select();
    }, 0);
    return () => window.clearTimeout(timer);
  }, [editingTabId]);

  useEffect(() => {
    if (!tabMenu) return;
    if (!tabs.some((tab) => tab.id === tabMenu.tabId)) {
      setTabMenu(null);
    }
  }, [tabs, tabMenu]);

  const tabMenuTarget = useMemo(
    () => (tabMenu ? tabs.find((tab) => tab.id === tabMenu.tabId) ?? null : null),
    [tabs, tabMenu],
  );

  const closeTabMenu = useCallback(() => {
    setTabMenu(null);
  }, []);

  const openTabMenu = useCallback(
    (event: ReactMouseEvent, tabId: string) => {
      event.preventDefault();
      event.stopPropagation();
      setActiveTab(tabId);
      const hostRect = tabMenuHostRef.current?.getBoundingClientRect();
      const pos = clampContextMenuPosition(
        hostRect ? event.clientX - hostRect.left : event.clientX,
        hostRect ? event.clientY - hostRect.top : event.clientY,
        hostRect ? { width: hostRect.width, height: hostRect.height } : undefined,
      );
      setTabMenu({ tabId, x: pos.x, y: pos.y });
    },
    [setActiveTab],
  );

  const beginRenameTab = useCallback((tab: WorkspaceTab) => {
    skipRenameCommitRef.current = false;
    setEditingTabId(tab.id);
    setEditingTabName(tab.name);
    setTabMenu(null);
  }, []);

  const handleLayoutChange = useCallback(
    (layout: string) => {
      if (!activeTabId) return;
      void updateTab(activeTabId, { layout, panel_sizes: null });
      setLayoutMenuOpen(false);
    },
    [activeTabId, updateTab],
  );

  const handleCreateTab = useCallback(() => {
    setWorkspaceError(null);
    void createTab().catch((error) => {
      setWorkspaceError(error instanceof Error ? error.message : "Failed to create tab");
    });
  }, [createTab]);

  const handleDeleteTab = useCallback(
    (id: string) => {
      setWorkspaceError(null);
      void deleteTab(id).catch((error) => {
        setWorkspaceError(error instanceof Error ? error.message : "Failed to delete tab");
      });
    },
    [deleteTab],
  );

  const cancelRenameTab = useCallback(() => {
    skipRenameCommitRef.current = true;
    setEditingTabId(null);
    setEditingTabName("");
  }, []);

  const commitRenameTab = useCallback(async () => {
    skipRenameCommitRef.current = false;
    if (!editingTabId) return;
    const normalized = editingTabName.trim().replace(/\s+/g, " ");
    if (!normalized) {
      setWorkspaceError("Tab name cannot be empty");
      return;
    }
    const safeName = normalized.slice(0, 48);
    setWorkspaceError(null);
    setEditingTabId(null);
    setEditingTabName("");
    try {
      await updateTab(editingTabId, { name: safeName });
    } catch (error) {
      setWorkspaceError(error instanceof Error ? error.message : "Failed to rename tab");
    }
  }, [editingTabId, editingTabName, updateTab]);

  const handleRenameInputBlur = useCallback(() => {
    if (skipRenameCommitRef.current) {
      skipRenameCommitRef.current = false;
      return;
    }
    void commitRenameTab();
  }, [commitRenameTab]);

  const handleDuplicateTab = useCallback(
    async (tab: WorkspaceTab) => {
      setWorkspaceError(null);
      try {
        const duplicated = await createTab(`${tab.name} Copy`);
        if (!duplicated?.id) return;
        await updateTab(duplicated.id, {
          layout: tab.layout,
          panes: tab.panes.map((pane) => ({ ...pane })),
        });
        setActiveTab(duplicated.id);
      } catch (error) {
        setWorkspaceError(error instanceof Error ? error.message : "Failed to duplicate tab");
      }
    },
    [createTab, setActiveTab, updateTab],
  );

  const handleCloseOtherTabs = useCallback(
    async (keepTabId: string) => {
      const otherTabs = tabs.filter((tab) => tab.id !== keepTabId);
      if (otherTabs.length === 0) return;
      setWorkspaceError(null);
      try {
        for (const tab of otherTabs) {
          await deleteTab(tab.id);
        }
        setActiveTab(keepTabId);
      } catch (error) {
        setWorkspaceError(error instanceof Error ? error.message : "Failed to close other tabs");
      }
    },
    [deleteTab, setActiveTab, tabs],
  );

  return {
    layoutMenuOpen,
    setLayoutMenuOpen,
    workspaceError,
    tabMenu,
    tabMenuTarget,
    editingTabId,
    editingTabName,
    setEditingTabName,
    layoutMenuRef,
    tabMenuHostRef,
    tabRenameInputRef,
    closeTabMenu,
    openTabMenu,
    beginRenameTab,
    handleLayoutChange,
    handleCreateTab,
    handleDeleteTab,
    cancelRenameTab,
    commitRenameTab,
    handleRenameInputBlur,
    handleDuplicateTab,
    handleCloseOtherTabs,
  };
}

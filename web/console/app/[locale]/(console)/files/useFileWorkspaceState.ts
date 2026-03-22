"use client";

import { useCallback, useState } from "react";

export type WorkspaceClipboardMode = "copy" | "cut";

export type WorkspaceClipboardItem = {
  name: string;
  path: string;
  is_dir: boolean;
};

export type WorkspaceClipboard<ScopeID extends string = string> = {
  mode: WorkspaceClipboardMode;
  ownerID: ScopeID;
  items: WorkspaceClipboardItem[];
};

export type WorkspaceContextMenuState<Entry> = {
  x: number;
  y: number;
  entry: Entry | null;
  names: string[];
  targetDirPath: string;
};

type ContextMenuSize = {
  width?: number;
  height?: number;
  padding?: number;
  boundsWidth?: number;
  boundsHeight?: number;
};

type OpenContextMenuArgs<Entry> = Omit<WorkspaceContextMenuState<Entry>, "x" | "y">;

export function useFileWorkspaceState<Entry, ScopeID extends string = string>() {
  const [selectedEntries, setSelectedEntries] = useState<Set<string>>(new Set());
  const [clipboard, setClipboard] = useState<WorkspaceClipboard<ScopeID> | null>(null);
  const [contextMenu, setContextMenu] = useState<WorkspaceContextMenuState<Entry> | null>(null);

  const toggleSelected = useCallback((name: string) => {
    setSelectedEntries((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else {
        next.add(name);
      }
      return next;
    });
  }, []);

  const toggleSelectAll = useCallback((allNames: string[]) => {
    setSelectedEntries((prev) => {
      if (prev.size === allNames.length) {
        return new Set();
      }
      return new Set(allNames);
    });
  }, []);

  const clearSelection = useCallback(() => {
    setSelectedEntries(new Set());
  }, []);

  const selectOnly = useCallback((name: string) => {
    setSelectedEntries(new Set([name]));
  }, []);

  const setSelectedNames = useCallback((names: string[]) => {
    setSelectedEntries(new Set(names));
  }, []);

  const selectionNamesFromEntry = useCallback((entryName: string): string[] => {
    if (selectedEntries.has(entryName) && selectedEntries.size > 0) {
      return Array.from(selectedEntries);
    }
    return [entryName];
  }, [selectedEntries]);

  const setClipboardItems = useCallback((
    mode: WorkspaceClipboardMode,
    ownerID: ScopeID,
    items: WorkspaceClipboardItem[],
  ) => {
    if (!ownerID || items.length === 0) {
      return;
    }
    setClipboard({
      mode,
      ownerID,
      items,
    });
  }, []);

  const clearClipboard = useCallback(() => {
    setClipboard(null);
  }, []);

  const openContextMenu = useCallback((
    x: number,
    y: number,
    state: OpenContextMenuArgs<Entry>,
    size?: ContextMenuSize,
  ) => {
    const menuWidth = size?.width ?? 240;
    const menuHeight = size?.height ?? 320;
    const padding = size?.padding ?? 8;
    const maxWidth = size?.boundsWidth ?? window.innerWidth;
    const maxHeight = size?.boundsHeight ?? window.innerHeight;
    const clampedX = Math.max(padding, Math.min(x, maxWidth - menuWidth - padding));
    const clampedY = Math.max(padding, Math.min(y, maxHeight - menuHeight - padding));

    setContextMenu({
      x: clampedX,
      y: clampedY,
      ...state,
    });
  }, []);

  const closeContextMenu = useCallback(() => {
    setContextMenu(null);
  }, []);

  return {
    selectedEntries,
    toggleSelected,
    toggleSelectAll,
    clearSelection,
    selectOnly,
    setSelectedNames,
    selectionNamesFromEntry,
    clipboard,
    setClipboardItems,
    clearClipboard,
    contextMenu,
    openContextMenu,
    closeContextMenu,
  };
}

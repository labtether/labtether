"use client";

import { useCallback, useEffect, useRef } from "react";
import type React from "react";
import type { FileEntry } from "../../../hooks/useFiles";

type ContextMenuState = {
  entry: FileEntry | null;
  names: string[];
  targetDirPath: string;
};

type OpenContextMenuFn = (
  x: number,
  y: number,
  state: ContextMenuState,
  size?: {
    width?: number;
    height?: number;
    boundsWidth?: number;
    boundsHeight?: number;
  },
) => void;

type UseFilesContextMenuInteractionsArgs = {
  currentPath: string;
  selectedEntries: Set<string>;
  selectionNamesFromEntry: (entryName: string) => string[];
  selectOnly: (name: string) => void;
  fullPathForName: (name: string) => string;
  openContextMenu: OpenContextMenuFn;
  closeContextMenu: () => void;
};

export function useFilesContextMenuInteractions({
  currentPath,
  selectedEntries,
  selectionNamesFromEntry,
  selectOnly,
  fullPathForName,
  openContextMenu,
  closeContextMenu,
}: UseFilesContextMenuInteractionsArgs) {
  const workspaceMenuHostRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const closeMenuOnOutsidePointerDown = (event: MouseEvent | TouchEvent) => {
      const host = workspaceMenuHostRef.current;
      const target = event.target;
      if (!(target instanceof Node)) {
        closeContextMenu();
        return;
      }
      if (host && host.contains(target)) {
        return;
      }
      closeContextMenu();
    };
    const closeMenu = () => closeContextMenu();
    window.addEventListener("mousedown", closeMenuOnOutsidePointerDown, true);
    window.addEventListener("touchstart", closeMenuOnOutsidePointerDown, true);
    window.addEventListener("resize", closeMenu);
    return () => {
      window.removeEventListener("mousedown", closeMenuOnOutsidePointerDown, true);
      window.removeEventListener("touchstart", closeMenuOnOutsidePointerDown, true);
      window.removeEventListener("resize", closeMenu);
    };
  }, [closeContextMenu]);

  const resolveContextMenuPosition = useCallback((clientX: number, clientY: number) => {
    const hostRect = workspaceMenuHostRef.current?.getBoundingClientRect();
    return {
      x: hostRect ? clientX - hostRect.left : clientX,
      y: hostRect ? clientY - hostRect.top : clientY,
      boundsWidth: hostRect?.width,
      boundsHeight: hostRect?.height,
    };
  }, []);

  const handleEntryContextMenu = useCallback((event: React.MouseEvent, entry: FileEntry) => {
    event.preventDefault();
    event.stopPropagation();
    const names = selectionNamesFromEntry(entry.name);
    if (!selectedEntries.has(entry.name) || selectedEntries.size <= 1) {
      selectOnly(entry.name);
    }
    const entryPath = fullPathForName(entry.name);
    const targetDirPath = entry.is_dir ? entryPath : currentPath;
    const menuPosition = resolveContextMenuPosition(event.clientX, event.clientY);
    openContextMenu(
      menuPosition.x,
      menuPosition.y,
      {
        entry,
        names,
        targetDirPath,
      },
      {
        width: 240,
        height: 320,
        boundsWidth: menuPosition.boundsWidth,
        boundsHeight: menuPosition.boundsHeight,
      },
    );
  }, [currentPath, fullPathForName, openContextMenu, resolveContextMenuPosition, selectOnly, selectedEntries, selectionNamesFromEntry]);

  const handleBackgroundContextMenu = useCallback((event: React.MouseEvent) => {
    event.preventDefault();
    const menuPosition = resolveContextMenuPosition(event.clientX, event.clientY);
    openContextMenu(
      menuPosition.x,
      menuPosition.y,
      {
        entry: null,
        names: Array.from(selectedEntries),
        targetDirPath: currentPath,
      },
      {
        width: 240,
        height: 320,
        boundsWidth: menuPosition.boundsWidth,
        boundsHeight: menuPosition.boundsHeight,
      },
    );
  }, [currentPath, openContextMenu, resolveContextMenuPosition, selectedEntries]);

  return {
    workspaceMenuHostRef,
    handleEntryContextMenu,
    handleBackgroundContextMenu,
  };
}

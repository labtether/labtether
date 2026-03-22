"use client";

import { useCallback, useEffect, useRef } from "react";
import type React from "react";
import { joinPath } from "../../files/fileWorkspaceUtils";
import type { FileEntry } from "../../files/fileOpsClient";

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

type UseFilesTabContextMenuInteractionsArgs = {
  currentPath: string;
  selectedEntries: Set<string>;
  selectionNamesFromEntry: (entryName: string) => string[];
  selectOnly: (name: string) => void;
  openContextMenu: OpenContextMenuFn;
  closeContextMenu: () => void;
};

export function useFilesTabContextMenuInteractions({
  currentPath,
  selectedEntries,
  selectionNamesFromEntry,
  selectOnly,
  openContextMenu,
  closeContextMenu,
}: UseFilesTabContextMenuInteractionsArgs) {
  const filesMenuHostRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const closeMenu = () => closeContextMenu();
    const onEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        closeContextMenu();
      }
    };

    window.addEventListener("click", closeMenu);
    window.addEventListener("resize", closeMenu);
    window.addEventListener("scroll", closeMenu, true);
    window.addEventListener("keydown", onEscape);

    return () => {
      window.removeEventListener("click", closeMenu);
      window.removeEventListener("resize", closeMenu);
      window.removeEventListener("scroll", closeMenu, true);
      window.removeEventListener("keydown", onEscape);
    };
  }, [closeContextMenu]);

  const handleEntryContextMenu = useCallback((event: React.MouseEvent, entry: FileEntry) => {
    event.preventDefault();
    event.stopPropagation();

    const names = selectionNamesFromEntry(entry.name);
    if (!selectedEntries.has(entry.name) || selectedEntries.size <= 1) {
      selectOnly(entry.name);
    }

    const entryPath = joinPath(currentPath, entry.name);
    const hostRect = filesMenuHostRef.current?.getBoundingClientRect();
    const menuX = hostRect ? event.clientX - hostRect.left : event.clientX;
    const menuY = hostRect ? event.clientY - hostRect.top : event.clientY;

    openContextMenu(
      menuX,
      menuY,
      {
        entry,
        names,
        targetDirPath: entry.is_dir ? entryPath : currentPath,
      },
      {
        width: 220,
        height: 280,
        boundsWidth: hostRect?.width,
        boundsHeight: hostRect?.height,
      },
    );
  }, [currentPath, openContextMenu, selectOnly, selectedEntries, selectionNamesFromEntry]);

  const handleBackgroundContextMenu = useCallback((event: React.MouseEvent) => {
    event.preventDefault();

    const hostRect = filesMenuHostRef.current?.getBoundingClientRect();
    const menuX = hostRect ? event.clientX - hostRect.left : event.clientX;
    const menuY = hostRect ? event.clientY - hostRect.top : event.clientY;

    openContextMenu(
      menuX,
      menuY,
      {
        entry: null,
        names: Array.from(selectedEntries),
        targetDirPath: currentPath,
      },
      {
        width: 220,
        height: 280,
        boundsWidth: hostRect?.width,
        boundsHeight: hostRect?.height,
      },
    );
  }, [currentPath, openContextMenu, selectedEntries]);

  return {
    filesMenuHostRef,
    handleEntryContextMenu,
    handleBackgroundContextMenu,
  };
}

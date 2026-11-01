"use client";

import { useCallback } from "react";
import type React from "react";
import { joinPath } from "../../files/fileWorkspaceUtils";
import type { FileEntry } from "../../files/fileOpsClient";

type ContextMenuState = {
  entry: FileEntry | null;
  names: string[];
  targetDirPath: string;
};

type UseFilesTabContextMenuInteractionsArgs = {
  currentPath: string;
  selectedEntries: Set<string>;
  selectionNamesFromEntry: (entryName: string) => string[];
  selectOnly: (name: string) => void;
  openContextMenu: (x: number, y: number, state: ContextMenuState) => void;
};

/** Right-click on a row (it becomes the pick unless it is one of several picks) or on the folder background. */
export function useFilesTabContextMenuInteractions({
  currentPath,
  selectedEntries,
  selectionNamesFromEntry,
  selectOnly,
  openContextMenu,
}: UseFilesTabContextMenuInteractionsArgs) {
  const handleContextMenu = useCallback((event: React.MouseEvent, entry: FileEntry | null) => {
    event.preventDefault();
    event.stopPropagation();
    if (!entry) {
      openContextMenu(event.clientX, event.clientY, { entry: null, names: Array.from(selectedEntries), targetDirPath: currentPath });
      return;
    }
    const names = selectionNamesFromEntry(entry.name);
    if (!selectedEntries.has(entry.name) || selectedEntries.size <= 1) selectOnly(entry.name);
    openContextMenu(event.clientX, event.clientY, {
      entry,
      names,
      targetDirPath: entry.is_dir ? joinPath(currentPath, entry.name) : currentPath,
    });
  }, [currentPath, openContextMenu, selectOnly, selectedEntries, selectionNamesFromEntry]);

  return { handleContextMenu };
}

"use client";

import { useCallback } from "react";
import { useTranslations } from "next-intl";
import type { FileEntry } from "../../files/fileOpsClient";
import { joinPath } from "../../files/fileWorkspaceUtils";
import { menuActions } from "../../files/filesListModel";
import { PointMenu, toMenuItems, useMenuLabels, type MenuItemSpec } from "../../files/FileMenus";
import type { WorkspaceClipboard, WorkspaceContextMenuState } from "../../files/useFileWorkspaceState";

// The item menu of a device's Files tab (row ⋯ and right-click). Changes show only after "Allow changes".

export type FilesTabMenuHandlers = {
  open: (entry: FileEntry) => void;
  download: (entries: FileEntry[]) => void;
  rename: (entry: FileEntry) => void;
  copyNames: (names: string[]) => void;
  cutNames: (names: string[]) => void;
  pasteTo: (dirPath: string) => void;
  remove: (entries: FileEntry[]) => void;
  newFolder: () => void;
  uploadHere: (dirPath: string) => void;
  refresh: () => void;
};

/** menuFor(entry or null, the names it acts on, the folder a paste or upload goes to). */
export function useFilesTabMenu({ entries, currentPath, writeEnabled, clipboard, h }: {
  entries: FileEntry[];
  currentPath: string;
  writeEnabled: boolean;
  clipboard: WorkspaceClipboard | null;
  h: FilesTabMenuHandlers;
}) {
  const label = useMenuLabels();
  return useCallback((entry: FileEntry | null, names: string[], dirPath: string): MenuItemSpec[] => {
    const picked = entries.filter((e) => names.includes(e.name));
    const slots = menuActions({
      entry,
      count: names.length,
      hasFiles: picked.some((e) => !e.is_dir),
      canPaste: Boolean(clipboard?.items.length),
      canWrite: writeEnabled,
    });
    return toMenuItems(slots, {
      open: entry ? () => h.open(entry) : undefined,
      download: () => h.download(picked),
      rename: entry ? () => h.rename(entry) : undefined,
      copy: () => h.copyNames(names),
      cut: () => h.cutNames(names),
      paste: () => h.pasteTo(dirPath),
      copyPath: entry ? () => void navigator.clipboard?.writeText(joinPath(currentPath, entry.name)) : undefined,
      delete: () => h.remove(picked),
      newFolder: h.newFolder,
      uploadHere: () => h.uploadHere(dirPath),
      refresh: h.refresh,
    }, (a) => label(a, { count: names.length, name: entry?.name ?? "", dir: Boolean(entry?.is_dir) }));
  }, [clipboard, currentPath, entries, h, label, writeEnabled]);
}

export function FilesContextMenu({ contextMenu, menuFor, onClose }: {
  contextMenu: WorkspaceContextMenuState<FileEntry> | null;
  menuFor: (entry: FileEntry | null, names: string[], dirPath: string) => MenuItemSpec[];
  onClose: () => void;
}) {
  const t = useTranslations("files.menu");
  const items = contextMenu
    ? menuFor(contextMenu.entry, contextMenu.entry ? contextMenu.names : [], contextMenu.targetDirPath)
    : [];
  return (
    <PointMenu
      at={contextMenu ? { x: contextMenu.x, y: contextMenu.y } : null}
      label={t("label")}
      items={items}
      onClose={onClose}
    />
  );
}

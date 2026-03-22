"use client";

import { useCallback } from "react";
import type { FileEntry } from "../../../hooks/useFiles";
import {
  applyWorkspaceClipboard,
  buildWorkspaceClipboardItems,
} from "./fileWorkspaceClipboard";
import type {
  WorkspaceClipboard,
  WorkspaceClipboardItem,
  WorkspaceClipboardMode,
} from "./useFileWorkspaceState";

type MutationOptions = {
  refresh?: boolean;
};

type EntryMutationFn = (
  srcPath: string,
  dstPath: string,
  options?: MutationOptions,
) => Promise<boolean>;

type UseFilesClipboardActionsArgs = {
  target: string;
  entries: FileEntry[];
  fullPathForName: (name: string) => string;
  setClipboardItems: (
    mode: WorkspaceClipboardMode,
    ownerID: string,
    items: WorkspaceClipboardItem[],
  ) => void;
  clipboard: WorkspaceClipboard<string> | null;
  copyEntry: EntryMutationFn;
  renameEntry: EntryMutationFn;
  listDir: (assetId: string, path: string, hidden?: boolean) => Promise<void>;
  currentPath: string;
  clearClipboard: () => void;
  setErrorMessage: (message: string | null) => void;
};

export function useFilesClipboardActions({
  target,
  entries,
  fullPathForName,
  setClipboardItems,
  clipboard,
  copyEntry,
  renameEntry,
  listDir,
  currentPath,
  clearClipboard,
  setErrorMessage,
}: UseFilesClipboardActionsArgs) {
  const setClipboardFromNames = useCallback((mode: WorkspaceClipboardMode, names: string[]) => {
    if (!target || names.length === 0) return;
    const items = buildWorkspaceClipboardItems({
      entries,
      names,
      getName: (entry) => entry.name,
      getPath: (entry) => fullPathForName(entry.name),
      getIsDir: (entry) => entry.is_dir,
    });
    if (items.length === 0) return;
    setClipboardItems(mode, target, items);
  }, [entries, fullPathForName, setClipboardItems, target]);

  const pasteClipboardToDir = useCallback(async (targetDirPath: string) => {
    if (!clipboard || !target) return;
    const outcome = await applyWorkspaceClipboard({
      clipboard,
      ownerID: target,
      targetDirPath,
      copyItem: (srcPath, dstPath) => copyEntry(srcPath, dstPath, { refresh: false }),
      moveItem: (srcPath, dstPath) => renameEntry(srcPath, dstPath, { refresh: false }),
    });
    if (outcome.status === "owner_mismatch") {
      setErrorMessage("Clipboard items belong to a different device. Copy/cut again on this device.");
      return;
    }
    if (outcome.status !== "applied") {
      return;
    }

    await listDir(target, currentPath);
    if (!outcome.failed && outcome.mode === "cut") {
      clearClipboard();
    }
  }, [clearClipboard, clipboard, copyEntry, currentPath, listDir, renameEntry, setErrorMessage, target]);

  return {
    setClipboardFromNames,
    pasteClipboardToDir,
  };
}

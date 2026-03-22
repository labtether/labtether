"use client";

import { useCallback, type Dispatch, type SetStateAction } from "react";
import { copyFilePath, renameFilePath, type FileEntry } from "../../files/fileOpsClient";
import {
  applyWorkspaceClipboard,
  buildWorkspaceClipboardItems,
} from "../../files/fileWorkspaceClipboard";
import { joinPath } from "../../files/fileWorkspaceUtils";
import type {
  WorkspaceClipboard,
  WorkspaceClipboardItem,
  WorkspaceClipboardMode,
} from "../../files/useFileWorkspaceState";

type SetClipboardItemsFn = (
  mode: WorkspaceClipboardMode,
  ownerID: string,
  items: WorkspaceClipboardItem[],
) => void;

type UseFilesTabClipboardActionsArgs = {
  nodeId: string;
  entries: FileEntry[];
  currentPath: string;
  clipboard: WorkspaceClipboard | null;
  setClipboardItems: SetClipboardItemsFn;
  clearClipboard: () => void;
  writeEnabled: boolean;
  listDir: (path: string) => Promise<void>;
  setActionBusy: Dispatch<SetStateAction<boolean>>;
  setError: Dispatch<SetStateAction<string | null>>;
  setActionMessage: Dispatch<SetStateAction<string | null>>;
};

function toErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

export function useFilesTabClipboardActions({
  nodeId,
  entries,
  currentPath,
  clipboard,
  setClipboardItems,
  clearClipboard,
  writeEnabled,
  listDir,
  setActionBusy,
  setError,
  setActionMessage,
}: UseFilesTabClipboardActionsArgs) {
  const copyPath = useCallback(async (srcPath: string, dstPath: string) => {
    await copyFilePath(nodeId, srcPath, dstPath, {
      fallbackError: (status) => `copy failed (${status})`,
    });
  }, [nodeId]);

  const renamePath = useCallback(async (oldPath: string, newPath: string) => {
    await renameFilePath(nodeId, oldPath, newPath, {
      fallbackError: (status) => `rename failed (${status})`,
    });
  }, [nodeId]);

  const setClipboardFromNames = useCallback((mode: WorkspaceClipboardMode, names: string[]) => {
    const items = buildWorkspaceClipboardItems({
      entries,
      names,
      getName: (entry) => entry.name,
      getPath: (entry) => joinPath(currentPath, entry.name),
      getIsDir: (entry) => entry.is_dir,
    });
    if (items.length === 0) {
      return;
    }
    setClipboardItems(mode, nodeId, items);
  }, [currentPath, entries, nodeId, setClipboardItems]);

  const pasteClipboardTo = useCallback(async (targetDirPath: string) => {
    if (!clipboard || !writeEnabled) return;
    setActionBusy(true);
    setError(null);
    setActionMessage(null);
    try {
      let failureMessage: string | null = null;
      const outcome = await applyWorkspaceClipboard({
        clipboard,
        ownerID: nodeId,
        targetDirPath,
        copyItem: async (srcPath, dstPath) => {
          try {
            await copyPath(srcPath, dstPath);
            return true;
          } catch (err) {
            failureMessage = toErrorMessage(err, "paste failed");
            return false;
          }
        },
        moveItem: async (srcPath, dstPath) => {
          try {
            await renamePath(srcPath, dstPath);
            return true;
          } catch (err) {
            failureMessage = toErrorMessage(err, "paste failed");
            return false;
          }
        },
      });
      if (outcome.status === "owner_mismatch") {
        setError("Clipboard items belong to a different device. Copy/cut again on this device.");
        return;
      }
      if (outcome.status !== "applied") {
        return;
      }
      if (outcome.failed) {
        setError(failureMessage ?? "paste failed");
        return;
      }

      if (outcome.mode === "copy" && outcome.copied > 0) {
        setActionMessage(`Copied ${outcome.copied} item${outcome.copied === 1 ? "" : "s"}`);
      } else if (outcome.mode === "cut" && outcome.moved > 0) {
        setActionMessage(`Moved ${outcome.moved} item${outcome.moved === 1 ? "" : "s"}`);
        clearClipboard();
      } else if (outcome.skipped > 0) {
        setActionMessage(outcome.skipped === 1
          ? "Item is already in this location."
          : "Items are already in this location.");
      }
      await listDir(currentPath);
    } catch (err) {
      setError(toErrorMessage(err, "paste failed"));
    } finally {
      setActionBusy(false);
    }
  }, [
    clearClipboard,
    clipboard,
    copyPath,
    currentPath,
    listDir,
    nodeId,
    renamePath,
    setActionBusy,
    setActionMessage,
    setError,
    writeEnabled,
  ]);

  const handleCopyEntry = useCallback((entry: FileEntry) => {
    setClipboardFromNames("copy", [entry.name]);
    setActionMessage(`Copied ${entry.name} to clipboard`);
  }, [setClipboardFromNames, setActionMessage]);

  const handleCutEntry = useCallback((entry: FileEntry) => {
    setClipboardFromNames("cut", [entry.name]);
    setActionMessage(`Cut ${entry.name} to clipboard`);
  }, [setClipboardFromNames, setActionMessage]);

  const handleCopyNames = useCallback((names: string[]) => {
    setClipboardFromNames("copy", names);
    setActionMessage(`Copied ${names.length} item${names.length === 1 ? "" : "s"} to clipboard`);
  }, [setClipboardFromNames, setActionMessage]);

  const handleCutNames = useCallback((names: string[]) => {
    setClipboardFromNames("cut", names);
    setActionMessage(`Cut ${names.length} item${names.length === 1 ? "" : "s"} to clipboard`);
  }, [setClipboardFromNames, setActionMessage]);

  const handlePasteTo = useCallback((targetDirPath: string) => {
    void pasteClipboardTo(targetDirPath);
  }, [pasteClipboardTo]);

  return {
    setClipboardFromNames,
    pasteClipboardTo,
    handleCopyEntry,
    handleCutEntry,
    handleCopyNames,
    handleCutNames,
    handlePasteTo,
  };
}

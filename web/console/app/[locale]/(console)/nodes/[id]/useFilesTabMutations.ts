"use client";

import { useCallback, useState, type RefObject } from "react";
import {
  createDirectory,
  deleteFilePath,
  downloadFileBlob,
  renameFilePath,
  uploadFileViaFetch,
  type FileEntry,
} from "../../files/fileOpsClient";
import { joinPath, triggerBlobDownload } from "../../files/fileWorkspaceUtils";

const MAX_UPLOAD_SIZE = 512 * 1024 * 1024; // 512MB

type EntryTarget = {
  name: string;
  path: string;
};

type UseFilesTabMutationsArgs = {
  nodeId: string;
};

type ListDirFn = (path: string) => Promise<void>;

type PathAndListDir = {
  currentPath: string;
  listDir: ListDirFn;
};

type PathAndWrite = {
  currentPath: string;
  writeEnabled: boolean;
};

type CreateFolderArgs = PathAndListDir & {
  writeEnabled: boolean;
};

type UploadSelectedFilesArgs = PathAndListDir & {
  writeEnabled: boolean;
  uploadDestinationPath: string;
  uploadInputRef: RefObject<HTMLInputElement | null>;
};

function toErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

export function useFilesTabMutations({ nodeId }: UseFilesTabMutationsArgs) {
  const [actionBusy, setActionBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [newFolderName, setNewFolderName] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<EntryTarget | null>(null);
  const [renameTarget, setRenameTarget] = useState<EntryTarget | null>(null);
  const [renameName, setRenameName] = useState("");

  const downloadEntry = useCallback(async (entry: FileEntry, currentPath: string) => {
    if (entry.is_dir) return;
    setActionBusy(true);
    setError(null);
    setActionMessage(null);
    try {
      const filePath = joinPath(currentPath, entry.name);
      const blob = await downloadFileBlob(nodeId, filePath, {
        fallbackError: (status) => `download failed (${status})`,
      });
      triggerBlobDownload(blob, entry.name);
      setActionMessage(`Downloaded ${entry.name}`);
    } catch (err) {
      setError(toErrorMessage(err, "download failed"));
    } finally {
      setActionBusy(false);
    }
  }, [nodeId]);

  const createFolder = useCallback(async ({
    currentPath,
    listDir,
    writeEnabled,
  }: CreateFolderArgs) => {
    const name = newFolderName.trim();
    if (!writeEnabled || name === "") return;
    setActionBusy(true);
    setError(null);
    setActionMessage(null);
    try {
      const path = joinPath(currentPath, name);
      await createDirectory(nodeId, path, {
        fallbackError: (status) => `mkdir failed (${status})`,
      });
      setNewFolderName("");
      setActionMessage(`Created folder ${name}`);
      await listDir(currentPath);
    } catch (err) {
      setError(toErrorMessage(err, "create folder failed"));
    } finally {
      setActionBusy(false);
    }
  }, [newFolderName, nodeId]);

  const uploadSelectedFiles = useCallback(async ({
    currentPath,
    listDir,
    writeEnabled,
    uploadDestinationPath,
    uploadInputRef,
  }: UploadSelectedFilesArgs) => {
    if (!writeEnabled) return;
    const selectedFiles = uploadInputRef.current?.files;
    if (!selectedFiles || selectedFiles.length === 0) return;

    setActionBusy(true);
    setError(null);
    setActionMessage(null);

    try {
      for (const file of Array.from(selectedFiles)) {
        if (file.size > MAX_UPLOAD_SIZE) {
          throw new Error(`"${file.name}" exceeds the 512 MB upload limit.`);
        }
        const destinationPath = joinPath(uploadDestinationPath, file.name);
        await uploadFileViaFetch(nodeId, destinationPath, file, {
          fallbackError: (status) => `upload failed (${status})`,
        });
      }
      if (uploadInputRef.current) {
        uploadInputRef.current.value = "";
      }
      setActionMessage(`Uploaded ${selectedFiles.length} file${selectedFiles.length === 1 ? "" : "s"}`);
      await listDir(currentPath);
    } catch (err) {
      setError(toErrorMessage(err, "upload failed"));
    } finally {
      setActionBusy(false);
    }
  }, [nodeId]);

  const renameEntry = useCallback((entry: FileEntry, { currentPath, writeEnabled }: PathAndWrite) => {
    if (!writeEnabled) return;
    setRenameTarget({ name: entry.name, path: joinPath(currentPath, entry.name) });
    setRenameName(entry.name);
  }, []);

  const commitRename = useCallback(async ({ currentPath, listDir }: PathAndListDir) => {
    if (!renameTarget) return;
    const nextName = renameName.trim();
    if (nextName === "" || nextName === renameTarget.name) {
      setRenameTarget(null);
      setRenameName("");
      return;
    }
    setActionBusy(true);
    setError(null);
    setActionMessage(null);
    try {
      const newPath = joinPath(currentPath, nextName);
      await renameFilePath(nodeId, renameTarget.path, newPath, {
        fallbackError: (status) => `rename failed (${status})`,
      });
      setActionMessage(`Renamed ${renameTarget.name} to ${nextName}`);
      setRenameTarget(null);
      setRenameName("");
      await listDir(currentPath);
    } catch (err) {
      setError(toErrorMessage(err, "rename failed"));
    } finally {
      setActionBusy(false);
    }
  }, [nodeId, renameName, renameTarget]);

  const cancelRename = useCallback(() => {
    setRenameTarget(null);
    setRenameName("");
  }, []);

  const deleteEntry = useCallback((entry: FileEntry, { currentPath, writeEnabled }: PathAndWrite) => {
    if (!writeEnabled) return;
    setDeleteTarget({ name: entry.name, path: joinPath(currentPath, entry.name) });
  }, []);

  const commitDelete = useCallback(async ({ currentPath, listDir }: PathAndListDir) => {
    if (!deleteTarget) return;
    setActionBusy(true);
    setError(null);
    setActionMessage(null);
    try {
      await deleteFilePath(nodeId, deleteTarget.path, {
        fallbackError: (status) => `delete failed (${status})`,
      });
      setActionMessage(`Deleted ${deleteTarget.name}`);
      setDeleteTarget(null);
      await listDir(currentPath);
    } catch (err) {
      setError(toErrorMessage(err, "delete failed"));
    } finally {
      setActionBusy(false);
    }
  }, [deleteTarget, nodeId]);

  return {
    actionBusy,
    setActionBusy,
    error,
    setError,
    actionMessage,
    setActionMessage,
    newFolderName,
    setNewFolderName,
    deleteTarget,
    setDeleteTarget,
    renameTarget,
    renameName,
    setRenameName,
    downloadEntry,
    createFolder,
    uploadSelectedFiles,
    renameEntry,
    commitRename,
    cancelRename,
    deleteEntry,
    commitDelete,
  };
}

"use client";

import { useCallback, useState } from "react";
import {
  createDirectory,
  deleteFilePath,
  downloadFileBlob,
  renameFilePath,
  uploadFileViaFetch,
  type FileEntry,
} from "../../files/fileOpsClient";
import { triggerBlobDownload } from "../../files/fileWorkspaceUtils";
import { joinPath } from "../../files/remotePaths";

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
  name: string;
};

type UploadFilesArgs = PathAndListDir & {
  files: File[];
  writeEnabled: boolean;
  uploadDestinationPath: string;
};

function toErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

export function useFilesTabMutations({ nodeId }: UseFilesTabMutationsArgs) {
  const [actionBusy, setActionBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [deleteTargets, setDeleteTargets] = useState<EntryTarget[] | null>(null);
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
    name: rawName,
  }: CreateFolderArgs) => {
    const name = rawName.trim();
    if (!writeEnabled || name === "") return;
    setActionBusy(true);
    setError(null);
    setActionMessage(null);
    try {
      const path = joinPath(currentPath, name);
      await createDirectory(nodeId, path, {
        fallbackError: (status) => `mkdir failed (${status})`,
      });
      setActionMessage(`Created folder ${name}`);
      await listDir(currentPath);
    } catch (err) {
      setError(toErrorMessage(err, "create folder failed"));
    } finally {
      setActionBusy(false);
    }
  }, [nodeId]);

  /** Uploads the files as given. The Files tab asks before it passes files that would replace others. */
  const uploadFiles = useCallback(async ({
    files: selectedFiles,
    currentPath,
    listDir,
    writeEnabled,
    uploadDestinationPath,
  }: UploadFilesArgs) => {
    if (!writeEnabled || selectedFiles.length === 0) return;

    setActionBusy(true);
    setError(null);
    setActionMessage(null);

    try {
      for (const file of selectedFiles) {
        if (file.size > MAX_UPLOAD_SIZE) {
          throw new Error(`"${file.name}" exceeds the 512 MB upload limit.`);
        }
        const destinationPath = joinPath(uploadDestinationPath, file.name);
        await uploadFileViaFetch(nodeId, destinationPath, file, {
          fallbackError: (status) => `upload failed (${status})`,
        });
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
      setRenameTarget(null);
      setRenameName("");
      setError(toErrorMessage(err, "rename failed"));
    } finally {
      setActionBusy(false);
    }
  }, [nodeId, renameName, renameTarget]);

  const cancelRename = useCallback(() => {
    setRenameTarget(null);
    setRenameName("");
  }, []);

  const deleteEntries = useCallback((entries: FileEntry[], { currentPath, writeEnabled }: PathAndWrite) => {
    if (!writeEnabled || entries.length === 0) return;
    setDeleteTargets(entries.map((entry) => ({ name: entry.name, path: joinPath(currentPath, entry.name) })));
  }, []);

  const commitDelete = useCallback(async ({ currentPath, listDir }: PathAndListDir) => {
    if (!deleteTargets?.length) return;
    setActionBusy(true);
    setError(null);
    setActionMessage(null);
    let deleted = 0;
    let failure: string | null = null;
    for (const target of deleteTargets) {
      try {
        await deleteFilePath(nodeId, target.path, {
          fallbackError: (status) => `delete failed (${status})`,
        });
        deleted += 1;
      } catch (err) {
        failure = toErrorMessage(err, "delete failed");
        break;
      }
    }
    setDeleteTargets(null);
    if (deleted > 0) await listDir(currentPath);
    if (failure) setError(failure);
    else setActionMessage(deleted === 1 ? `Deleted ${deleteTargets[0].name}` : `Deleted ${deleted} items`);
    setActionBusy(false);
  }, [deleteTargets, nodeId]);

  return {
    actionBusy,
    setActionBusy,
    error,
    setError,
    actionMessage,
    setActionMessage,
    deleteTargets,
    setDeleteTargets,
    renameTarget,
    renameName,
    setRenameName,
    downloadEntry,
    createFolder,
    uploadFiles,
    renameEntry,
    commitRename,
    cancelRename,
    deleteEntries,
    commitDelete,
  };
}

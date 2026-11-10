"use client";

import { useCallback, useEffect, useMemo } from "react";
import { useTranslations } from "next-intl";
import { useFiles, type FileEntry, type SortField } from "../../../hooks/useFiles";
import { unifiedListFiles, type UnifiedFileEntry } from "./fileOpsClient";
import {
  isOutsideRoot, isTopFolder, isWindowsPlatform, joinPath, normalizePath, outsideBasePath, pathStyleOf, samePath, typedPath,
  type PathStyle,
} from "./remotePaths";
import type { FolderEntry } from "./uploadClashes";
import { useConnectionBrowser } from "./useConnectionBrowser";
import { sourceKey, sourceOf, type FileSource, type FileTab } from "./useFileTabsState";

// One shape for the folder a Files tab shows, whether it browses a device through its agent (useFiles) or a saved
// connection (useConnectionBrowser). Paths in names-based calls are inside the open folder.

export type Progress = { fileName: string; loaded: number; total: number };

export type FilesSourceApi = {
  key: string;
  source: FileSource | null;
  /** The tab browses a device whose agent is not connected. */
  waiting: boolean;
  currentPath: string;
  rootPath: string;
  /** How this source writes paths: "windows" for a Windows device, "posix" for every other device and connection. */
  pathStyle: PathStyle;
  entries: FileEntry[];
  loading: boolean;
  error: string | null;
  setError: (message: string | null) => void;
  showHidden: boolean;
  setShowHidden: (next: boolean) => void;
  sortField: SortField;
  sortDir: "asc" | "desc";
  toggleSort: (field: SortField) => void;
  refresh: () => void;
  goTo: (path: string) => void;
  goUp: () => void;
  canUp: boolean;
  openFile: (entry: FileEntry) => void;
  download: (names: string[]) => void;
  upload: (files: File[], dir: string) => Promise<void>;
  /** Reads a folder with its hidden files and leaves the open folder as it is. */
  listFolder: (dir: string) => Promise<FolderEntry[]>;
  /** File names on this source match without regard to case (a Windows device). */
  namesIgnoreCase: boolean;
  createDir: (name: string) => Promise<void>;
  remove: (names: string[]) => Promise<void>;
  rename: (from: string, to: string) => Promise<void>;
  copyPath: (src: string, dst: string) => Promise<boolean>;
  movePath: (src: string, dst: string) => Promise<boolean>;
  preview: { name: string; content: string } | null;
  closePreview: () => void;
  uploadProgress: Progress | null;
  cancelUpload: () => void;
  downloadProgress: Progress | null;
  cancelDownload: () => void;
};

const toEntry = (e: UnifiedFileEntry): FileEntry => ({ name: e.name, is_dir: e.is_dir, size: e.size, mod_time: e.mod_time ?? "", mode: e.mode ?? "" });

export function useFilesSource(tab: FileTab | null): FilesSourceApi {
  const files = useFiles();
  const { target, setTarget, connectedAgentIds, listDir } = files;
  const source = useMemo(() => sourceOf(tab), [tab]);
  const conn = useConnectionBrowser(source?.type === "connection" ? source : null);
  const isConn = source?.type === "connection";

  const agentId = source?.type === "agent" ? source.assetId : "";
  const agentConnected = agentId !== "" && connectedAgentIds.has(agentId);
  // useFiles drops a target that is not among its connected assets, so wait until the status lists the device too.
  const agentReady = agentConnected && files.assets.some((a) => a.id === agentId);

  // The agent tab follows the active tab and its agent's connection; it opens at the device's home folder.
  useEffect(() => {
    setTarget(agentReady ? agentId : "");
  }, [agentId, agentReady, setTarget]);
  useEffect(() => {
    if (target && target === agentId) void listDir(target, "~");
  }, [target, agentId, listDir]);

  const connEntries = useMemo(() => conn.entries.map(toEntry), [conn.entries]);
  const currentPath = isConn ? conn.currentPath : files.currentPath;
  const entries = isConn ? connEntries : files.entries;
  const rootPath = isConn ? conn.rootPath : files.rootPath;
  const at = useCallback((name: string) => joinPath(currentPath, name), [currentPath]);

  const download = useCallback((names: string[]) => {
    for (const name of names) {
      const entry = entries.find((e) => e.name === name);
      if (!entry || entry.is_dir) continue;
      if (isConn) conn.downloadFile(at(name));
      else files.downloadFile(at(name));
    }
  }, [at, conn, entries, files, isConn]);

  const openFile = useCallback((entry: FileEntry) => {
    if (entry.is_dir) {
      if (isConn) conn.navigate(entry.name);
      else files.navigate(entry.name);
      return;
    }
    if (!isConn && files.isPreviewable(entry.name, entry.size)) void files.previewFile(at(entry.name), entry.name);
    else download([entry.name]);
  }, [at, conn, download, files, isConn]);

  const upload = useCallback(async (list: File[], dir: string) => {
    for (const file of list) {
      if (isConn) await conn.uploadFile(joinPath(dir, file.name), file);
      else await files.uploadFile(file, dir);
    }
  }, [conn, files, isConn]);

  const listFolder = useCallback(async (dir: string): Promise<FolderEntry[]> => {
    if (!source) throw new Error("no source");
    return (await unifiedListFiles(source, dir, { showHidden: true })).entries;
  }, [source]);
  const agentPlatform = isConn ? "" : files.assets.find((a) => a.id === agentId)?.platform ?? "";
  const namesIgnoreCase = isWindowsPlatform(agentPlatform);
  // The agent's own answer decides the style; the platform only stands in until the first folder is read.
  const pathStyle = pathStyleOf(rootPath === "~" ? currentPath : rootPath, agentPlatform);

  const remove = useCallback(async (names: string[]) => {
    if (names.length === 0) return;
    if (names.length === 1) {
      if (isConn) await conn.deleteEntry(at(names[0]));
      else await files.deleteEntry(at(names[0]));
      return;
    }
    if (isConn) await conn.deleteSelected(names);
    else await files.deleteSelected(names);
  }, [at, conn, files, isConn]);

  const rename = useCallback(async (from: string, to: string) => {
    if (isConn) await conn.renameEntry(at(from), at(to));
    else await files.renameEntry(at(from), at(to));
  }, [at, conn, files, isConn]);

  const copyPath = useCallback(async (src: string, dst: string) => {
    if (!isConn) return files.copyEntry(src, dst, { refresh: false });
    await conn.copyEntry(src, dst);
    return true;
  }, [conn, files, isConn]);

  const movePath = useCallback(async (src: string, dst: string) => {
    if (!isConn) return files.renameEntry(src, dst, { refresh: false });
    await conn.renameEntry(src, dst);
    return true;
  }, [conn, files, isConn]);

  const refresh = useCallback(() => {
    if (isConn) conn.refresh();
    else if (target) void listDir(target, files.currentPath);
  }, [conn, files.currentPath, isConn, listDir, target]);

  const setShowHidden = useCallback((next: boolean) => {
    if (isConn) {
      conn.setShowHidden(next);
      return;
    }
    files.setShowHidden(next);
    if (target) void listDir(target, files.currentPath, next);
  }, [conn, files, isConn, listDir, target]);

  // A typed path outside the agent's folder used to fall back to that folder in silence; now it says why.
  const tBar = useTranslations("files.bar");
  const goToAgent = useCallback((path: string) => {
    const wanted = typedPath(path, pathStyle);
    if (isOutsideRoot(wanted, files.rootPath)) {
      files.setErrorMessage(tBar("outsideRoot", { path: normalizePath(wanted, pathStyle), root: files.rootPath }));
      return;
    }
    files.navigateToPath(wanted);
  }, [files, pathStyle, tBar]);
  const refused = outsideBasePath(files.error);
  const agentError = refused === null ? files.error : tBar("outsideShared", { path: refused });

  const atRoot = rootPath !== "~" && rootPath !== "" && samePath(currentPath, rootPath);
  const canUp = !atRoot && !isTopFolder(currentPath);

  return {
    key: sourceKey(source),
    source,
    waiting: source?.type === "agent" && !agentConnected,
    currentPath,
    rootPath,
    pathStyle,
    entries,
    loading: isConn ? conn.loading : files.loading || (agentConnected && target !== agentId),
    error: isConn ? conn.error : agentError,
    setError: isConn ? conn.setError : files.setErrorMessage,
    showHidden: isConn ? conn.showHidden : files.showHidden,
    setShowHidden,
    sortField: isConn ? conn.sortField : files.sortField,
    sortDir: isConn ? conn.sortDir : files.sortDir,
    toggleSort: isConn ? conn.toggleSort : files.toggleSort,
    refresh,
    goTo: isConn ? conn.navigateToPath : goToAgent,
    goUp: isConn ? conn.navigateUp : files.navigateUp,
    canUp,
    openFile,
    download,
    upload,
    listFolder,
    namesIgnoreCase,
    createDir: isConn ? conn.createDir : files.createDir,
    remove,
    rename,
    copyPath,
    movePath,
    preview: isConn ? null : files.previewContent,
    closePreview: files.closePreview,
    uploadProgress: isConn ? null : files.uploadProgress,
    cancelUpload: files.cancelUpload,
    downloadProgress: isConn ? null : files.downloadProgress,
    cancelDownload: files.cancelDownload,
  };
}

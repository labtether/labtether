"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { FileSource } from "./useFileTabsState";
import type { SortField } from "../../../hooks/useFiles";
import {
  unifiedListFiles,
  unifiedCreateDirectory,
  unifiedDeleteFilePath,
  unifiedRenameFilePath,
  unifiedDownloadFileBlob,
  unifiedUploadFileViaFetch,
  unifiedCopyFilePath,
  type UnifiedFileEntry,
} from "./fileOpsClient";
import { joinPath, parentPath } from "./fileWorkspaceUtils";

// Re-export for consumers
export type { UnifiedFileEntry };

type SortDir = "asc" | "desc";

function sortEntries(items: UnifiedFileEntry[], field: SortField, dir: SortDir): UnifiedFileEntry[] {
  return [...items].sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
    let cmp = 0;
    switch (field) {
      case "name":
        cmp = a.name.localeCompare(b.name);
        break;
      case "size":
        cmp = a.size - b.size;
        break;
      case "mod_time":
        cmp = (a.mod_time || "").localeCompare(b.mod_time || "");
        break;
    }
    return dir === "asc" ? cmp : -cmp;
  });
}

export function useConnectionBrowser(source: FileSource | null) {
  const [currentPath, setCurrentPath] = useState("/");
  const [entries, setEntries] = useState<UnifiedFileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showHidden, setShowHiddenState] = useState(false);
  const [sortField, setSortField] = useState<SortField>("name");
  const [sortDir, setSortDir] = useState<SortDir>("asc");

  const sourceRef = useRef(source);
  sourceRef.current = source;
  const sortFieldRef = useRef(sortField);
  sortFieldRef.current = sortField;
  const sortDirRef = useRef(sortDir);
  sortDirRef.current = sortDir;
  const fetchIdRef = useRef(0);

  const fetchEntries = useCallback(
    async (path: string, hidden?: boolean) => {
      const src = sourceRef.current;
      if (!src) return;
      const id = ++fetchIdRef.current;
      setLoading(true);
      setError(null);
      try {
        const result = await unifiedListFiles(src, path, {
          showHidden: hidden,
        });
        if (id !== fetchIdRef.current) return; // stale response
        const resolvedPath = result.path ?? path;
        setCurrentPath(resolvedPath);
        setEntries(sortEntries(result.entries, sortFieldRef.current, sortDirRef.current));
      } catch (err) {
        if (id !== fetchIdRef.current) return; // stale error
        setError(err instanceof Error ? err.message : "Failed to list files");
        setEntries([]);
      } finally {
        if (id === fetchIdRef.current) setLoading(false);
      }
    },
    [],
  );

  // Reload when source identity changes
  const prevSourceKey = useRef<string | null>(null);
  useEffect(() => {
    if (!source) return;
    const key = source.type === "agent" ? `a:${source.assetId}` : `c:${source.connectionId}`;
    if (prevSourceKey.current === key) return;
    prevSourceKey.current = key;
    const initialPath = source.type === "agent" ? "~" : "/";
    void fetchEntries(initialPath, showHidden);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [source, fetchEntries]);

  const navigate = useCallback(
    (name: string) => {
      void fetchEntries(joinPath(currentPath, name), showHidden);
    },
    [currentPath, fetchEntries, showHidden],
  );

  const navigateUp = useCallback(() => {
    void fetchEntries(parentPath(currentPath), showHidden);
  }, [currentPath, fetchEntries, showHidden]);

  const navigateToPath = useCallback(
    (path: string) => {
      void fetchEntries(path, showHidden);
    },
    [fetchEntries, showHidden],
  );

  const refresh = useCallback(() => {
    void fetchEntries(currentPath, showHidden);
  }, [currentPath, fetchEntries, showHidden]);

  const handleShowHiddenChange = useCallback(
    (next: boolean) => {
      setShowHiddenState(next);
      void fetchEntries(currentPath, next);
    },
    [currentPath, fetchEntries],
  );

  const toggleSort = useCallback((field: SortField) => {
    setSortField((prevField) => {
      const newDir: SortDir = prevField === field && sortDirRef.current === "asc" ? "desc" : "asc";
      setSortDir(newDir);
      setEntries((prev) => sortEntries(prev, field, newDir));
      return field;
    });
  }, []);

  const createDir = useCallback(
    async (name: string) => {
      const src = sourceRef.current;
      if (!src) return;
      const id = fetchIdRef.current;
      try {
        await unifiedCreateDirectory(src, joinPath(currentPath, name));
        if (id === fetchIdRef.current) void fetchEntries(currentPath, showHidden);
      } catch (err) {
        if (id === fetchIdRef.current) setError(err instanceof Error ? err.message : "Failed to create directory");
      }
    },
    [currentPath, fetchEntries, showHidden],
  );

  const deleteEntry = useCallback(
    async (path: string) => {
      const src = sourceRef.current;
      if (!src) return;
      const id = fetchIdRef.current;
      try {
        await unifiedDeleteFilePath(src, path);
        if (id === fetchIdRef.current) void fetchEntries(currentPath, showHidden);
      } catch (err) {
        if (id === fetchIdRef.current) setError(err instanceof Error ? err.message : "Failed to delete");
      }
    },
    [currentPath, fetchEntries, showHidden],
  );

  const deleteSelected = useCallback(
    async (names: string[]) => {
      const src = sourceRef.current;
      if (!src) return;
      const id = fetchIdRef.current;
      const results = await Promise.allSettled(
        names.map((name) =>
          unifiedDeleteFilePath(src, joinPath(currentPath, name)),
        ),
      );
      if (id !== fetchIdRef.current) return;
      const failures = results.filter((r) => r.status === "rejected");
      if (failures.length > 0) {
        const msg = failures.length === names.length
          ? "Failed to delete selected items"
          : `${failures.length} of ${names.length} items failed to delete`;
        setError(msg);
      }
      void fetchEntries(currentPath, showHidden);
    },
    [currentPath, fetchEntries, showHidden],
  );

  const renameEntry = useCallback(
    async (oldPath: string, newPath: string) => {
      const src = sourceRef.current;
      if (!src) return;
      const id = fetchIdRef.current;
      try {
        await unifiedRenameFilePath(src, oldPath, newPath);
        if (id === fetchIdRef.current) void fetchEntries(currentPath, showHidden);
      } catch (err) {
        if (id === fetchIdRef.current) setError(err instanceof Error ? err.message : "Failed to rename");
      }
    },
    [currentPath, fetchEntries, showHidden],
  );

  const uploadFile = useCallback(
    async (destPath: string, file: File) => {
      const src = sourceRef.current;
      if (!src) return;
      const id = fetchIdRef.current;
      try {
        await unifiedUploadFileViaFetch(src, destPath, file);
        if (id === fetchIdRef.current) void fetchEntries(currentPath, showHidden);
      } catch (err) {
        if (id === fetchIdRef.current) setError(err instanceof Error ? err.message : "Failed to upload");
      }
    },
    [currentPath, fetchEntries, showHidden],
  );

  const copyEntry = useCallback(
    async (srcPath: string, dstPath: string) => {
      const src = sourceRef.current;
      if (!src) return;
      const id = fetchIdRef.current;
      try {
        await unifiedCopyFilePath(src, srcPath, dstPath);
        if (id === fetchIdRef.current) void fetchEntries(currentPath, showHidden);
      } catch (err) {
        if (id === fetchIdRef.current) setError(err instanceof Error ? err.message : "Failed to copy");
      }
    },
    [currentPath, fetchEntries, showHidden],
  );

  const downloadFile = useCallback((path: string) => {
    const src = sourceRef.current;
    if (!src) return;
    const id = fetchIdRef.current; // guard against source changes
    void (async () => {
      try {
        const blob = await unifiedDownloadFileBlob(src, path);
        if (id !== fetchIdRef.current) return;
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = path.split("/").pop() ?? "download";
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(url);
      } catch (err) {
        if (id !== fetchIdRef.current) return;
        setError(err instanceof Error ? err.message : "Failed to download");
      }
    })();
  }, []);

  const rootPath = source?.type === "agent" ? "~" : "/";

  return {
    currentPath,
    rootPath,
    entries,
    loading,
    error,
    showHidden,
    sortField,
    sortDir,
    navigate,
    navigateUp,
    navigateToPath,
    refresh,
    setShowHidden: handleShowHiddenChange,
    toggleSort,
    createDir,
    deleteEntry,
    deleteSelected,
    renameEntry,
    uploadFile,
    copyEntry,
    downloadFile,
    setError,
  };
}

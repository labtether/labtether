"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { listFiles, type FileEntry } from "../../files/fileOpsClient";
import { joinPath, parentPath } from "../../files/fileWorkspaceUtils";

type UseFilesTabDirectoryStateArgs = {
  nodeId: string;
  setError: (error: string | null) => void;
};

export function useFilesTabDirectoryState({ nodeId, setError }: UseFilesTabDirectoryStateArgs) {
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [currentPath, setCurrentPath] = useState("~");
  const [pathInput, setPathInput] = useState("~");
  const [loading, setLoading] = useState(false);
  const [showHidden, setShowHidden] = useState(false);

  const showHiddenRef = useRef(showHidden);
  const currentPathRef = useRef(currentPath);
  const listRequestSeqRef = useRef(0);
  const latestListRequestRef = useRef(0);

  useEffect(() => {
    showHiddenRef.current = showHidden;
  }, [showHidden]);

  useEffect(() => {
    currentPathRef.current = currentPath;
  }, [currentPath]);

  const sortedEntries = useMemo(() => {
    return [...entries].sort((left, right) => {
      if (left.is_dir !== right.is_dir) {
        return left.is_dir ? -1 : 1;
      }
      return left.name.localeCompare(right.name, undefined, { sensitivity: "base" });
    });
  }, [entries]);

  const listDir = useCallback(async (path: string) => {
    const requestID = ++listRequestSeqRef.current;
    latestListRequestRef.current = requestID;
    setLoading(true);
    setError(null);

    try {
      const payload = await listFiles(nodeId, path, {
        showHidden: showHiddenRef.current,
        fallbackError: (status) => `failed to list files (${status})`,
      });

      if (latestListRequestRef.current !== requestID) {
        return;
      }

      const resolvedPath = payload.path?.trim() || path;
      setEntries(payload.entries);
      setCurrentPath(resolvedPath);
      setPathInput(resolvedPath);
    } catch (err) {
      if (latestListRequestRef.current !== requestID) {
        return;
      }

      setEntries([]);
      setError(err instanceof Error ? err.message : "failed to list files");
    } finally {
      if (latestListRequestRef.current === requestID) {
        setLoading(false);
      }
    }
  }, [nodeId, setError]);

  useEffect(() => {
    void listDir(currentPathRef.current);
  }, [showHidden, listDir]);

  useEffect(() => {
    void listDir("~");
  }, [listDir]);

  const navigateUp = useCallback(() => {
    const nextPath = parentPath(currentPath, { fallbackRoot: "/" });
    void listDir(nextPath);
  }, [currentPath, listDir]);

  const navigateTo = useCallback((path: string) => {
    const nextPath = path.trim();
    if (nextPath === "") return;
    void listDir(nextPath);
  }, [listDir]);

  const openEntry = useCallback((entry: FileEntry) => {
    if (!entry.is_dir) return;
    void listDir(joinPath(currentPath, entry.name));
  }, [currentPath, listDir]);

  return {
    entries,
    sortedEntries,
    currentPath,
    pathInput,
    loading,
    showHidden,
    setShowHidden,
    setPathInput,
    listDir,
    navigateUp,
    navigateTo,
    openEntry,
  };
}

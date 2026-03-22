"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useFastStatus } from "../contexts/StatusContext";
import { useConnectedAgents } from "./useConnectedAgents";
import {
  clampPathToRoot,
  joinPath,
  normalizePath,
  parentPath,
  triggerBlobDownload,
} from "../[locale]/(console)/files/fileWorkspaceUtils";
import {
  copyFilePath,
  createDirectory,
  deleteFilePath,
  downloadFileResponse,
  listFiles,
  renameFilePath,
  startUploadFileXhr,
  type FileEntry,
} from "../[locale]/(console)/files/fileOpsClient";
export type { FileEntry } from "../[locale]/(console)/files/fileOpsClient";

export type SortField = "name" | "size" | "mod_time";
export type SortDir = "asc" | "desc";
type MutationOptions = {
  refresh?: boolean;
};

const TEXT_EXTENSIONS = new Set([
  "txt", "md", "log", "json", "yaml", "yml", "toml", "go", "ts", "tsx",
  "js", "jsx", "py", "sh", "bash", "conf", "cfg", "ini", "env", "csv",
  "xml", "html", "css", "sql", "rs", "c", "cpp", "h", "java", "rb",
  "swift",
]);
const MAX_UPLOAD_SIZE = 512 * 1024 * 1024; // 512MB

async function detectFilesystemRoot(assetId: string, showHidden: boolean): Promise<string | null> {
  try {
    const data = await listFiles(assetId, "/", {
      showHidden,
      fallbackError: (status) => `Failed (${status})`,
    });
    return normalizePath(data.path || "/");
  } catch {
    return null;
  }
}

export function useFiles() {
  const status = useFastStatus();
  const { connectedAgentIds, refreshConnected } = useConnectedAgents();
  const allAssets = status?.assets;
  const assets = useMemo(
    () => (allAssets ?? []).filter((asset) => connectedAgentIds.has(asset.id)),
    [allAssets, connectedAgentIds]
  );

  const [target, setTarget] = useState("");
  const [currentPath, setCurrentPath] = useState("~");
  const [rootPath, setRootPath] = useState("~");
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showHidden, setShowHidden] = useState(false);
  const [sortField, setSortField] = useState<SortField>("name");
  const [sortDir, setSortDir] = useState<SortDir>("asc");
  const [uploadProgress, setUploadProgress] = useState<{ fileName: string; loaded: number; total: number } | null>(null);
  const [previewContent, setPreviewContent] = useState<{ name: string; content: string } | null>(null);
  const [downloadProgress, setDownloadProgress] = useState<{ fileName: string; loaded: number; total: number } | null>(null);
  const uploadXhrRef = useRef<XMLHttpRequest | null>(null);
  const downloadAbortRef = useRef<AbortController | null>(null);
  const showHiddenRef = useRef(showHidden);
  const rootPathRef = useRef(rootPath);
  const listRequestSeqRef = useRef(0);
  const latestListRequestRef = useRef(0);

  useEffect(() => {
    showHiddenRef.current = showHidden;
  }, [showHidden]);

  useEffect(() => {
    rootPathRef.current = rootPath;
  }, [rootPath]);

  useEffect(() => {
    if (!target) {
      return;
    }
    if (assets.some((asset) => asset.id === target)) {
      return;
    }
    // Invalidate any in-flight list request tied to the now-unavailable target.
    latestListRequestRef.current = listRequestSeqRef.current + 1;
    setTarget("");
    setCurrentPath("~");
    setRootPath("~");
    setEntries([]);
  }, [assets, target]);

  const listDir = useCallback(
    async (assetId: string, path: string, hidden?: boolean) => {
      const requestedPath = normalizePath(path);
      const requestID = ++listRequestSeqRef.current;
      latestListRequestRef.current = requestID;
      setLoading(true);
      setError(null);
      try {
        const data = await listFiles(assetId, requestedPath, {
          showHidden: hidden ?? showHiddenRef.current,
          fallbackError: (status) => `Failed (${status})`,
        });
        if (latestListRequestRef.current !== requestID) {
          return;
        }
        const resolvedPath = normalizePath(data.path || requestedPath);
        setEntries(data.entries);
        setCurrentPath(resolvedPath);
        if (requestedPath === "~") {
          setRootPath(resolvedPath);
          void detectFilesystemRoot(assetId, hidden ?? showHiddenRef.current).then((detectedRoot) => {
            if (latestListRequestRef.current !== requestID || !detectedRoot) {
              return;
            }
            setRootPath(detectedRoot);
          });
        }
      } catch (err) {
        if (latestListRequestRef.current !== requestID) {
          return;
        }
        setError(err instanceof Error ? err.message : "Failed to list directory");
        setEntries([]);
      } finally {
        if (latestListRequestRef.current === requestID) {
          setLoading(false);
        }
      }
    },
    []
  );

  const navigate = useCallback(
    (dirName: string) => {
      if (!target) return;
      const current = normalizePath(currentPath);
      const joinedPath = joinPath(current, dirName);
      void listDir(target, clampPathToRoot(joinedPath, rootPathRef.current));
    },
    [target, currentPath, listDir]
  );

  const navigateUp = useCallback(() => {
    if (!target) return;
    const root = normalizePath(rootPathRef.current);
    const current = normalizePath(currentPath);
    if (current === root) return;
    const next = clampPathToRoot(parentPath(current), root);
    void listDir(target, next);
  }, [target, currentPath, listDir]);

  const navigateToPath = useCallback(
    (path: string) => {
      if (!target) return;
      void listDir(target, clampPathToRoot(path, rootPathRef.current));
    },
    [target, listDir]
  );

  const downloadFile = useCallback(
    (filePath: string) => {
      if (!target) return;
      const fileName = filePath.split("/").pop() || "download";

      const abort = new AbortController();
      downloadAbortRef.current = abort;
      setDownloadProgress({ fileName, loaded: 0, total: -1 });

      downloadFileResponse(target, filePath, { signal: abort.signal })
        .then(async (res) => {
          if (!res.ok) {
            setDownloadProgress(null);
            downloadAbortRef.current = null;
            setError(`Download failed (${res.status})`);
            return;
          }
          const clHeader = res.headers.get("content-length");
          const total = clHeader ? parseInt(clHeader, 10) : -1;
          const reader = res.body?.getReader();
          if (!reader) {
            setDownloadProgress(null);
            downloadAbortRef.current = null;
            const blob = await res.blob();
            triggerBlobDownload(blob, fileName);
            return;
          }

          const chunks: Uint8Array[] = [];
          let loaded = 0;
          for (;;) {
            const { done, value } = await reader.read();
            if (done) break;
            chunks.push(value);
            loaded += value.length;
            setDownloadProgress({ fileName, loaded, total });
          }
          setDownloadProgress(null);
          downloadAbortRef.current = null;
          const blobParts = chunks.map((chunk) => {
            const copied = new Uint8Array(chunk.byteLength);
            copied.set(chunk);
            return copied;
          });
          const blob = new Blob(blobParts);
          triggerBlobDownload(blob, fileName);
        })
        .catch((err) => {
          downloadAbortRef.current = null;
          setDownloadProgress(null);
          if (err instanceof DOMException && err.name === "AbortError") return;
          setError("Download failed: network error");
        });
    },
    [target]
  );

  const uploadFile = useCallback(
    (file: File, destPath: string): Promise<void> => {
      if (!target) return Promise.resolve();
      setError(null);

      if (file.size > MAX_UPLOAD_SIZE) {
        setError(`File "${file.name}" exceeds 512 MB limit (${(file.size / 1024 / 1024).toFixed(1)} MB)`);
        return Promise.resolve();
      }

      const uploadPath = joinPath(destPath, file.name);

      return new Promise<void>((resolve) => {
        const xhr = startUploadFileXhr(target, uploadPath, file, {
          onProgress: (e) => {
            if (e.lengthComputable) {
              setUploadProgress({ fileName: file.name, loaded: e.loaded, total: e.total });
            }
          },
          onLoad: (completedXhr) => {
            uploadXhrRef.current = null;
            setUploadProgress(null);
            if (completedXhr.status >= 200 && completedXhr.status < 300) {
              void listDir(target, currentPath);
            } else {
              try {
                const data = JSON.parse(completedXhr.responseText);
                setError(data.error || `Upload failed (${completedXhr.status})`);
              } catch {
                setError(`Upload failed (${completedXhr.status})`);
              }
            }
            resolve();
          },
          onError: () => {
            uploadXhrRef.current = null;
            setUploadProgress(null);
            setError("Upload failed: network error");
            resolve();
          },
          onAbort: () => {
            uploadXhrRef.current = null;
            setUploadProgress(null);
            resolve();
          },
        });

        uploadXhrRef.current = xhr;
      });
    },
    [target, currentPath, listDir]
  );

  const cancelUpload = useCallback(() => {
    if (uploadXhrRef.current) {
      uploadXhrRef.current.abort();
      uploadXhrRef.current = null;
    }
    setUploadProgress(null);
  }, []);

  const cancelDownload = useCallback(() => {
    if (downloadAbortRef.current) {
      downloadAbortRef.current.abort();
      downloadAbortRef.current = null;
    }
    setDownloadProgress(null);
  }, []);

  const createDir = useCallback(
    async (dirName: string) => {
      if (!target) return;
      setError(null);
      try {
        const newPath = joinPath(currentPath, dirName);
        await createDirectory(target, newPath, {
          fallbackError: (status) => `Mkdir failed (${status})`,
        });
        void listDir(target, currentPath);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Create directory failed");
      }
    },
    [target, currentPath, listDir]
  );

  const deleteEntry = useCallback(
    async (entryPath: string) => {
      if (!target) return;
      setError(null);
      try {
        await deleteFilePath(target, entryPath, {
          fallbackError: (status) => `Delete failed (${status})`,
        });
        void listDir(target, currentPath);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Delete failed");
      }
    },
    [target, currentPath, listDir]
  );

  const renameEntry = useCallback(
    async (oldPath: string, newPath: string, options?: MutationOptions): Promise<boolean> => {
      if (!target) return false;
      setError(null);
      try {
        await renameFilePath(target, oldPath, newPath, {
          fallbackError: (status) => `Rename failed (${status})`,
        });
        if (options?.refresh !== false) {
          void listDir(target, currentPath);
        }
        return true;
      } catch (err) {
        setError(err instanceof Error ? err.message : "Rename failed");
        return false;
      }
    },
    [target, currentPath, listDir]
  );

  const copyEntry = useCallback(
    async (srcPath: string, dstPath: string, options?: MutationOptions): Promise<boolean> => {
      if (!target) return false;
      setError(null);
      try {
        await copyFilePath(target, srcPath, dstPath, {
          fallbackError: (status) => `Copy failed (${status})`,
        });
        if (options?.refresh !== false) {
          void listDir(target, currentPath);
        }
        return true;
      } catch (err) {
        setError(err instanceof Error ? err.message : "Copy failed");
        return false;
      }
    },
    [target, currentPath, listDir]
  );

  const deleteSelected = useCallback(
    async (names: string[]) => {
      if (!target) return;
      setError(null);
      for (const name of names) {
        const fullPath = joinPath(currentPath, name);
        try {
          await deleteFilePath(target, fullPath, {
            fallbackError: (status) => `Delete failed (${status})`,
          });
        } catch (err) {
          setError(err instanceof Error ? err.message : "Delete failed");
          break;
        }
      }
      void listDir(target, currentPath);
    },
    [target, currentPath, listDir]
  );

  const isPreviewable = useCallback((name: string, size: number): boolean => {
    if (size > 1024 * 1024) return false; // 1MB limit
    const ext = name.includes(".") ? name.split(".").pop()?.toLowerCase() ?? "" : "";
    const baseName = name.toLowerCase();
    return TEXT_EXTENSIONS.has(ext) || baseName === "makefile" || baseName === "dockerfile";
  }, []);

  const previewFile = useCallback(
    async (filePath: string, fileName: string) => {
      if (!target) return;
      setError(null);
      try {
        const res = await downloadFileResponse(target, filePath);
        if (!res.ok) {
          throw new Error(`Preview failed (${res.status})`);
        }
        const text = await res.text();
        setPreviewContent({ name: fileName, content: text });
      } catch (err) {
        setError(err instanceof Error ? err.message : "Preview failed");
      }
    },
    [target]
  );

  const closePreview = useCallback(() => setPreviewContent(null), []);
  const setErrorMessage = useCallback((message: string | null) => setError(message), []);

  // Sort entries: directories first, then by selected field.
  const sortedEntries = useMemo(() => {
    return [...entries].sort((a, b) => {
      // Directories always come first.
      if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;

      let cmp = 0;
      switch (sortField) {
        case "name":
          cmp = a.name.localeCompare(b.name, undefined, { sensitivity: "base" });
          break;
        case "size":
          cmp = a.size - b.size;
          break;
        case "mod_time":
          cmp = a.mod_time.localeCompare(b.mod_time);
          break;
      }
      return sortDir === "asc" ? cmp : -cmp;
    });
  }, [entries, sortField, sortDir]);

  const toggleSort = useCallback(
    (field: SortField) => {
      if (sortField === field) {
        setSortDir((d) => (d === "asc" ? "desc" : "asc"));
      } else {
        setSortField(field);
        setSortDir("asc");
      }
    },
    [sortField]
  );

  return {
    assets,
    target,
    setTarget,
    connectedAgentIds,
    refreshConnected,
    currentPath,
    rootPath,
    entries: sortedEntries,
    loading,
    error,
    showHidden,
    setShowHidden,
    sortField,
    sortDir,
    toggleSort,
    listDir,
    navigate,
    navigateUp,
    navigateToPath,
    downloadFile,
    uploadFile,
    createDir,
    deleteEntry,
    renameEntry,
    copyEntry,
    uploadProgress,
    cancelUpload,
    downloadProgress,
    cancelDownload,
    deleteSelected,
    isPreviewable,
    previewFile,
    previewContent,
    closePreview,
    setErrorMessage,
  };
}

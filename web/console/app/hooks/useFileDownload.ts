"use client";

import { useCallback, useRef, useState } from "react";
import { downloadFileResponse } from "../[locale]/(console)/files/fileOpsClient";
import { triggerBlobDownload } from "../[locale]/(console)/files/fileWorkspaceUtils";

interface FileDownloadState {
  downloading: boolean;
  progress: number; // 0-100, -1 = indeterminate
  filename: string | null;
  error: string | null;
}

export function useFileDownload(nodeId: string) {
  const [state, setState] = useState<FileDownloadState>({
    downloading: false,
    progress: -1,
    filename: null,
    error: null,
  });
  const abortRef = useRef<AbortController | null>(null);

  const downloadFile = useCallback(
    async (remotePath: string) => {
      if (!nodeId || !remotePath.trim()) return;

      const filename =
        remotePath.split("/").pop() || remotePath.split("\\").pop() || "download";
      setState({ downloading: true, progress: -1, filename, error: null });

      try {
        abortRef.current = new AbortController();
        const res = await downloadFileResponse(nodeId, remotePath, {
          signal: abortRef.current.signal,
        });

        if (!res.ok) {
          const payload = await res.json().catch(() => ({ error: undefined }));
          throw new Error(
            payload.error || `Download failed (${res.status})`,
          );
        }

        const clHeader = res.headers.get("content-length");
        const total = clHeader ? parseInt(clHeader, 10) : -1;
        const reader = res.body?.getReader();

        if (!reader) {
          // Fallback: no streaming support
          const blob = await res.blob();
          triggerBlobDownload(blob, filename);
          setState({ downloading: false, progress: 100, filename, error: null });
          return;
        }

        const chunks: Uint8Array[] = [];
        let loaded = 0;
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          chunks.push(value);
          loaded += value.length;
          if (total > 0) {
            setState((prev) => ({
              ...prev,
              progress: Math.round((loaded / total) * 100),
            }));
          }
        }

        const blobParts = chunks.map((chunk) => {
          const copied = new Uint8Array(chunk.byteLength);
          copied.set(chunk);
          return copied;
        });
        const blob = new Blob(blobParts);
        triggerBlobDownload(blob, filename);

        setState({ downloading: false, progress: 100, filename, error: null });
      } catch (err) {
        if ((err as Error).name === "AbortError") {
          setState({ downloading: false, progress: -1, filename: null, error: null });
        } else {
          setState({
            downloading: false,
            progress: -1,
            filename: null,
            error: (err as Error).message,
          });
        }
      } finally {
        abortRef.current = null;
      }
    },
    [nodeId],
  );

  const cancel = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setState({ downloading: false, progress: -1, filename: null, error: null });
  }, []);

  return { ...state, downloadFile, cancel };
}

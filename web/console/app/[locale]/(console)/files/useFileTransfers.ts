"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  type FileTransfer,
  type StartTransferRequest,
  startTransfer,
  getTransfer,
  cancelTransfer as cancelTransferApi,
} from "./fileTransferClient";
import type { FileSource } from "./useFileTabsState";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function sourceToTransferFields(source: FileSource): { type: string; id: string } {
  if (source.type === "agent") {
    return { type: "agent", id: source.assetId };
  }
  return { type: "connection", id: source.connectionId };
}

function isActive(t: FileTransfer): boolean {
  return t.status === "pending" || t.status === "in_progress";
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

const BASE_POLL_INTERVAL = 1000;
const MAX_POLL_INTERVAL = 5000;

export function useFileTransfers() {
  const [transfers, setTransfers] = useState<FileTransfer[]>([]);
  const transfersRef = useRef(transfers);
  useEffect(() => { transfersRef.current = transfers; }, [transfers]);

  // --------------------------------------------------
  // Polling for active transfers with adaptive backoff
  // --------------------------------------------------

  useEffect(() => {
    let interval = BASE_POLL_INTERVAL;
    let consecutiveErrors = 0;
    let timer: ReturnType<typeof setTimeout>;

    const poll = async () => {
      const active = transfersRef.current.filter(isActive);
      if (active.length === 0) {
        interval = BASE_POLL_INTERVAL;
        consecutiveErrors = 0;
        timer = setTimeout(poll, interval);
        return;
      }

      const updates = await Promise.allSettled(
        active.map((t) => getTransfer(t.id)),
      );

      const failures = updates.filter((r) => r.status === "rejected").length;
      if (failures === updates.length) {
        consecutiveErrors++;
        interval = Math.min(BASE_POLL_INTERVAL * 2 ** consecutiveErrors, MAX_POLL_INTERVAL);
      } else {
        consecutiveErrors = 0;
        interval = BASE_POLL_INTERVAL;
      }

      setTransfers((current) => {
        const updated = new Map(current.map((t) => [t.id, t]));
        for (const result of updates) {
          if (result.status === "fulfilled") {
            updated.set(result.value.id, result.value);
          }
        }
        return Array.from(updated.values());
      });

      timer = setTimeout(poll, interval);
    };

    timer = setTimeout(poll, interval);
    return () => clearTimeout(timer);
  }, []);

  // --------------------------------------------------
  // Actions
  // --------------------------------------------------

  const startFileTransfer = useCallback(
    async (
      sourceFiles: string[],
      sourceSource: FileSource,
      destSource: FileSource,
      destPath: string,
    ) => {
      const src = sourceToTransferFields(sourceSource);
      const dst = sourceToTransferFields(destSource);

      for (const filePath of sourceFiles) {
        const req: StartTransferRequest = {
          source_type: src.type,
          source_id: src.id,
          source_path: filePath,
          dest_type: dst.type,
          dest_id: dst.id,
          dest_path: destPath,
        };
        try {
          const transfer = await startTransfer(req);
          setTransfers((prev) => [...prev, transfer]);
        } catch {
          // Surface a failed-client-side transfer entry so the UI shows the error.
          const failedEntry: FileTransfer = {
            id: crypto.randomUUID(),
            source_type: src.type,
            source_id: src.id,
            source_path: filePath,
            dest_type: dst.type,
            dest_id: dst.id,
            dest_path: destPath,
            file_name: filePath.split("/").pop() ?? filePath,
            bytes_transferred: 0,
            status: "failed",
            error: "Failed to initiate transfer",
          };
          setTransfers((prev) => [...prev, failedEntry]);
        }
      }
    },
    [],
  );

  const cancelFileTransfer = useCallback(async (transferId: string) => {
    try {
      await cancelTransferApi(transferId);
      // Optimistically mark as failed/cancelled.
      setTransfers((prev) =>
        prev.map((t) =>
          t.id === transferId ? { ...t, status: "failed", error: "Cancelled" } : t,
        ),
      );
    } catch {
      // Even if the API fails, mark it locally.
      setTransfers((prev) =>
        prev.map((t) =>
          t.id === transferId ? { ...t, status: "failed", error: "Cancel failed" } : t,
        ),
      );
    }
  }, []);

  const clearCompleted = useCallback(() => {
    setTransfers((prev) => prev.filter(isActive));
  }, []);

  return {
    transfers,
    startTransfer: startFileTransfer,
    cancelTransfer: cancelFileTransfer,
    clearCompleted,
  };
}

"use client";

import { useCallback, useState } from "react";
import { withCollector } from "../proxmoxFormatters";

export type TaskLogState = { text: string; loading: boolean; failed: boolean };

/**
 * Opens one Proxmox task log at a time under its row. Logs are kept once loaded, so closing and opening a row again
 * does not fetch twice. stop() asks Proxmox to stop a running task.
 */
export function useProxmoxTaskLog(collectorID: string) {
  const [openRef, setOpenRef] = useState<string | null>(null);
  const [logs, setLogs] = useState<Record<string, TaskLogState>>({});

  const toggle = useCallback(async (node: string, upid: string) => {
    const ref = `${node}/${upid}`;
    if (openRef === ref) {
      setOpenRef(null);
      return;
    }
    setOpenRef(ref);
    if (logs[ref] && !logs[ref].failed) return;
    setLogs((prev) => ({ ...prev, [ref]: { text: "", loading: true, failed: false } }));
    try {
      const response = await fetch(
        withCollector(`/api/proxmox/tasks/${encodeURIComponent(node)}/${encodeURIComponent(upid)}/log`, collectorID),
        { cache: "no-store", signal: AbortSignal.timeout(15_000) },
      );
      if (!response.ok) throw new Error("log");
      const data = (await response.json().catch(() => null)) as { log?: string } | null;
      setLogs((prev) => ({ ...prev, [ref]: { text: data?.log?.trim() ?? "", loading: false, failed: false } }));
    } catch {
      setLogs((prev) => ({ ...prev, [ref]: { text: "", loading: false, failed: true } }));
    }
  }, [collectorID, logs, openRef]);

  const stop = useCallback(async (node: string, upid: string): Promise<boolean> => {
    try {
      const response = await fetch(
        withCollector(`/api/proxmox/tasks/${encodeURIComponent(node)}/${encodeURIComponent(upid)}/stop`, collectorID),
        { method: "POST" },
      );
      return response.ok;
    } catch {
      return false;
    }
  }, [collectorID]);

  return { openRef, logs, toggle, stop };
}

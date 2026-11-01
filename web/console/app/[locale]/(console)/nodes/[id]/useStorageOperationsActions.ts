"use client";

import { useCallback, useState } from "react";
import { useTranslations } from "next-intl";

type RunProxmoxAction = (
  actionID: string,
  target: string,
  params?: Record<string, string>,
) => Promise<void> | void;

type UseStorageOperationsActionsArgs = {
  fetchInsights: () => Promise<void>;
  onRunProxmoxAction?: RunProxmoxAction;
  proxmoxCollectorID: string;
  storageNode: string;
};

export function useStorageOperationsActions({
  fetchInsights,
  onRunProxmoxAction,
  proxmoxCollectorID,
  storageNode,
}: UseStorageOperationsActionsArgs) {
  const t = useTranslations("devices-proxmox.pools");
  const [expandedTaskRef, setExpandedTaskRef] = useState<string | null>(null);
  const [taskLogLoadingRef, setTaskLogLoadingRef] = useState<string | null>(null);
  const [taskLogs, setTaskLogs] = useState<Record<string, string>>({});
  const [taskLogError, setTaskLogError] = useState<string | null>(null);
  const [localActionError, setLocalActionError] = useState<string | null>(null);

  const openTaskLog = useCallback(async (node: string, upid: string) => {
    const taskNode = node.trim();
    const taskUPID = upid.trim();
    if (taskNode === "" || taskUPID === "") return;

    const ref = `${taskNode}/${taskUPID}`;
    if (expandedTaskRef === ref) {
      setExpandedTaskRef(null);
      return;
    }

    setTaskLogError(null);
    if (taskLogs[ref]) {
      setExpandedTaskRef(ref);
      return;
    }

    setTaskLogLoadingRef(ref);
    setExpandedTaskRef(ref);
    try {
      const collectorQuery = proxmoxCollectorID !== ""
        ? `?collector_id=${encodeURIComponent(proxmoxCollectorID)}`
        : "";
      const res = await fetch(
        `/api/proxmox/tasks/${encodeURIComponent(taskNode)}/${encodeURIComponent(taskUPID)}/log${collectorQuery}`,
        { cache: "no-store", signal: AbortSignal.timeout(15_000) },
      );
      const payload = (await res.json().catch(() => null)) as { log?: string; error?: string } | null;
      if (!res.ok) {
        throw new Error(payload?.error || "");
      }
      setTaskLogs((prev) => ({
        ...prev,
        [ref]: payload?.log?.trim() ? payload.log : t("noLog"),
      }));
    } catch (err) {
      const reason = err instanceof Error ? err.message.trim() : "";
      setTaskLogError(reason ? t("logFailedWith", { reason }) : t("logFailed"));
      setTaskLogs((prev) => ({
        ...prev,
        [ref]: t("logFailed"),
      }));
    } finally {
      setTaskLogLoadingRef((prev) => (prev === ref ? null : prev));
    }
  }, [expandedTaskRef, taskLogs, proxmoxCollectorID, t]);

  const runPoolBackup = useCallback(async (
    poolName: string,
    kind: "vm" | "ct",
    vmid: number,
  ) => {
    if (!onRunProxmoxAction) return;
    if (storageNode.trim() === "") {
      setLocalActionError(t("noNode"));
      return;
    }

    setLocalActionError(null);
    const target = `${storageNode}/${vmid}`;
    const actionID = kind === "ct" ? "ct.backup" : "vm.backup";

    try {
      await Promise.resolve(onRunProxmoxAction(actionID, target, {
        storage: poolName,
        mode: "snapshot",
      }));
      void fetchInsights();
    } catch (err) {
      const reason = err instanceof Error ? err.message.trim() : "";
      setLocalActionError(reason ? t("backupFailedWith", { reason }) : t("backupFailed"));
    }
  }, [fetchInsights, onRunProxmoxAction, storageNode, t]);

  return {
    expandedTaskRef,
    taskLogLoadingRef,
    taskLogs,
    taskLogError,
    localActionError,
    openTaskLog,
    runPoolBackup,
  };
}

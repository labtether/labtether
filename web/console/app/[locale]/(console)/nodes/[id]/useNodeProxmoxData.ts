"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { useDocumentVisibility } from "../../../../hooks/useDocumentVisibility";
import {
  normalizeClusterStatusEntries,
  normalizeNetworkInterfaces,
  normalizeProxmoxDetails,
  type ClusterStatusEntry,
  type NetworkInterface,
  type ProxmoxDetails,
} from "./nodeDetailTypes";

/** The words for a queued Proxmox action, by action ID; others read "Sent to Proxmox". */
const ACTION_WORDS: Record<string, string> = {
  "vm.snapshot.rollback": "rollback",
  "ct.snapshot.rollback": "rollback",
  "vm.snapshot.delete": "snapshotDelete",
  "ct.snapshot.delete": "snapshotDelete",
  "vm.snapshot.create": "snapshot",
  "ct.snapshot.create": "snapshot",
  "vm.backup": "backup",
  "ct.backup": "backup",
};

type UseNodeProxmoxDataArgs = {
  activeTab: string;
  nodeId: string;
  isProxmoxAsset: boolean;
  isInfra: boolean;
  effectiveKind: string;
  proxmoxNode: string;
  proxmoxTarget: string;
  proxmoxCollectorHint: string;
};

export function useNodeProxmoxData({
  activeTab,
  nodeId,
  isProxmoxAsset,
  isInfra,
  effectiveKind,
  proxmoxNode,
  proxmoxTarget,
  proxmoxCollectorHint,
}: UseNodeProxmoxDataArgs) {
  const t = useTranslations("devices-proxmox.pve.actions");
  const [proxmoxActionRunning, setProxmoxActionRunning] = useState(false);
  const [proxmoxActionMessage, setProxmoxActionMessage] = useState<string | null>(null);
  const [proxmoxActionError, setProxmoxActionError] = useState<string | null>(null);

  const [proxmoxDetails, setProxmoxDetails] = useState<ProxmoxDetails | null>(null);
  const [proxmoxLoading, setProxmoxLoading] = useState(false);
  const [proxmoxError, setProxmoxError] = useState<string | null>(null);
  const [clusterStatus, setClusterStatus] = useState<ClusterStatusEntry[]>([]);
  const [networkInterfaces, setNetworkInterfaces] = useState<NetworkInterface[]>([]);
  const isDocumentVisible = useDocumentVisibility();
  const proxmoxFetchInFlightRef = useRef(false);

  const proxmoxCollectorID = useMemo(() => (
    proxmoxDetails?.collector_id?.trim()
    || proxmoxCollectorHint.trim()
    || ""
  ), [proxmoxCollectorHint, proxmoxDetails?.collector_id]);

  const fetchProxmoxDetails = useCallback(async (signal?: AbortSignal) => {
    if (!nodeId || !isProxmoxAsset) return;
    if (proxmoxFetchInFlightRef.current) return;
    proxmoxFetchInFlightRef.current = true;
    setProxmoxLoading(true);
    setProxmoxError(null);

    const attempt = async (): Promise<ProxmoxDetails> => {
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), 15_000);
      const combinedSignal = signal
        ? AbortSignal.any([signal, controller.signal])
        : controller.signal;
      try {
        const response = await fetch(
          `/api/proxmox/assets/${encodeURIComponent(nodeId)}/details`,
          { cache: "no-store", signal: combinedSignal },
        );
        const payload = normalizeProxmoxDetails(await response.json().catch(() => null));
        if (!response.ok) {
          throw new Error(payload.error || `failed to load proxmox details (${response.status})`);
        }
        return payload;
      } finally {
        clearTimeout(timeout);
      }
    };

    try {
      const payload = await attempt();
      if (!signal?.aborted) setProxmoxDetails(payload);
    } catch {
      if (signal?.aborted) return;
      await new Promise((resolve) => setTimeout(resolve, 2_000));
      if (signal?.aborted) return;
      try {
        const payload = await attempt();
        if (!signal?.aborted) setProxmoxDetails(payload);
      } catch (retryErr) {
        if (!signal?.aborted) {
          setProxmoxError(retryErr instanceof Error ? retryErr.message : "failed to load proxmox details");
          setProxmoxDetails(null);
        }
      }
    } finally {
      proxmoxFetchInFlightRef.current = false;
      setProxmoxLoading(false);
    }
  }, [isProxmoxAsset, nodeId]);

  const needsProxmoxDetails = activeTab === "proxmox"
    || (activeTab === "overview" && isInfra && effectiveKind === "node")
    || (activeTab === "storage" && isInfra && effectiveKind === "node");
  useEffect(() => {
    if (!needsProxmoxDetails || !nodeId || !isProxmoxAsset || !isDocumentVisible) return;
    const controller = new AbortController();
    void fetchProxmoxDetails(controller.signal);

    const interval = setInterval(() => {
      void fetchProxmoxDetails(controller.signal);
    }, 60_000);

    return () => {
      controller.abort();
      clearInterval(interval);
    };
  }, [needsProxmoxDetails, isDocumentVisible, isProxmoxAsset, nodeId, fetchProxmoxDetails]);

  const needsClusterData = activeTab === "proxmox" || (activeTab === "overview" && isInfra && effectiveKind === "node");
  useEffect(() => {
    if (!needsClusterData || !isProxmoxAsset || !isDocumentVisible) return;
    const controller = new AbortController();
    const collectorQuery = proxmoxCollectorID !== ""
      ? `?collector_id=${encodeURIComponent(proxmoxCollectorID)}`
      : "";

    void (async () => {
      try {
        const res = await fetch(`/api/proxmox/cluster/status${collectorQuery}`, { cache: "no-store", signal: controller.signal });
        if (res.ok) {
          const data = (await res.json().catch(() => null)) as { entries?: unknown } | null;
          if (!controller.signal.aborted) setClusterStatus(normalizeClusterStatusEntries(data?.entries));
        }
      } catch {
        // ignore request failures in polling view
      }
    })();

    if (proxmoxNode && (effectiveKind === "node" || effectiveKind === "qemu" || effectiveKind === "lxc")) {
      void (async () => {
        try {
          const res = await fetch(`/api/proxmox/nodes/${encodeURIComponent(proxmoxNode)}/network${collectorQuery}`, { cache: "no-store", signal: controller.signal });
          if (res.ok) {
            const data = (await res.json().catch(() => null)) as { interfaces?: unknown } | null;
            if (!controller.signal.aborted) setNetworkInterfaces(normalizeNetworkInterfaces(data?.interfaces));
          }
        } catch {
          // ignore request failures in polling view
        }
      })();
    }

    return () => { controller.abort(); };
  }, [needsClusterData, isDocumentVisible, isProxmoxAsset, proxmoxNode, effectiveKind, proxmoxCollectorID]);

  useEffect(() => {
    if (activeTab !== "settings" || !nodeId || !isProxmoxAsset) return;
    if (!isDocumentVisible) return;
    if (proxmoxCollectorID) return;
    const controller = new AbortController();
    void fetchProxmoxDetails(controller.signal);
    return () => { controller.abort(); };
  }, [activeTab, fetchProxmoxDetails, isDocumentVisible, isProxmoxAsset, nodeId, proxmoxCollectorID]);

  const runProxmoxQuickAction = useCallback(async (
    actionID: string,
    params?: Record<string, string>,
    targetOverride?: string,
  ) => {
    const target = (targetOverride ?? "").trim() || proxmoxTarget;
    if (!target) {
      setProxmoxActionError(t("noTarget"));
      return;
    }
    const actionParams: Record<string, string> = {
      ...(params ?? {}),
    };
    if (proxmoxCollectorID !== "") {
      actionParams.collector_id = proxmoxCollectorID;
    }
    setProxmoxActionRunning(true);
    setProxmoxActionError(null);
    setProxmoxActionMessage(null);
    try {
      const response = await fetch("/api/actions/execute", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          type: "connector_action",
          actor_id: "owner",
          connector_id: "proxmox",
          action_id: actionID,
          target,
          params: actionParams,
        }),
      });
      const payload = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) {
        throw new Error(payload.error || "");
      }
      setProxmoxActionMessage(t(`sent.${ACTION_WORDS[actionID] ?? "other"}`));
    } catch (err) {
      const reason = err instanceof Error ? err.message.trim() : "";
      setProxmoxActionError(reason ? t("failedWith", { reason }) : t("failed"));
    } finally {
      setProxmoxActionRunning(false);
    }
  }, [proxmoxCollectorID, proxmoxTarget, t]);

  return {
    proxmoxDetails,
    proxmoxLoading,
    proxmoxError,
    clusterStatus,
    networkInterfaces,
    proxmoxCollectorID,
    proxmoxActionRunning,
    proxmoxActionMessage,
    proxmoxActionError,
    fetchProxmoxDetails,
    runProxmoxQuickAction,
  };
}

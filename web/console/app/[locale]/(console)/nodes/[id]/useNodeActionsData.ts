"use client";

import { useTranslations } from "next-intl";
import { useCallback, useEffect, useState } from "react";
import type { ActionRun } from "../../../../console/models";

type UseNodeActionsDataArgs = {
  activeTab: string;
  nodeId: string;
};

export function useNodeActionsData({ activeTab, nodeId }: UseNodeActionsDataArgs) {
  const tMenu = useTranslations("devices.page.menu");
  const [actionRuns, setActionRuns] = useState<ActionRun[]>([]);
  const [actionsLoading, setActionsLoading] = useState(false);
  const [expandedActionId, setExpandedActionId] = useState<string | null>(null);

  const [nodeActionRunning, setNodeActionRunning] = useState(false);
  const [nodeActionMessage, setNodeActionMessage] = useState<string | null>(null);
  const [nodeActionError, setNodeActionError] = useState<string | null>(null);
  const [nodeNetworkActionRunning, setNodeNetworkActionRunning] = useState(false);
  const [nodeNetworkActionMessage, setNodeNetworkActionMessage] = useState<string | null>(null);
  const [nodeNetworkActionError, setNodeNetworkActionError] = useState<string | null>(null);

  useEffect(() => {
    if (activeTab !== "actions" || !nodeId) return;
    const controller = new AbortController();
    setActionsLoading(true);
    const load = async () => {
      try {
        const params = new URLSearchParams({ target: nodeId });
        const res = await fetch(`/api/actions/runs?${params.toString()}`, {
          cache: "no-store",
          signal: controller.signal,
        });
        if (!res.ok) {
          setActionRuns([]);
          return;
        }
        const data = (await res.json()) as { runs?: ActionRun[] };
        setActionRuns(data.runs ?? (Array.isArray(data) ? data as ActionRun[] : []));
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setActionRuns([]);
      } finally {
        if (!controller.signal.aborted) setActionsLoading(false);
      }
    };
    void load();
    return () => { controller.abort(); };
  }, [activeTab, nodeId]);

  /** Restart or shut down through the agent (POST /api/v2/assets/{id}/reboot|shutdown, proxied). */
  const runNodePower = useCallback(async (power: "restart" | "shutdown", name: string) => {
    if (!nodeId) return;
    setNodeActionRunning(true);
    setNodeActionError(null);
    setNodeActionMessage(null);
    const failed = power === "restart" ? "restartFailed" : "shutdownFailed";
    try {
      const response = await fetch(`/api/assets/${encodeURIComponent(nodeId)}/power`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action: power === "restart" ? "reboot" : "shutdown" }),
      });
      const payload = (await response.json().catch(() => ({}))) as { message?: string; error?: string };
      if (!response.ok) {
        const reason = (payload.message || payload.error || response.statusText || String(response.status)).trim();
        setNodeActionError(tMenu(failed, { name, reason }));
        return;
      }
      setNodeActionMessage(tMenu(power === "restart" ? "restarting" : "shuttingDown", { name }));
    } catch (err) {
      setNodeActionError(tMenu(failed, { name, reason: err instanceof Error ? err.message : "" }));
    } finally {
      setNodeActionRunning(false);
    }
  }, [nodeId, tMenu]);

  const runNodeNetworkAction = useCallback(async (
    action: "snapshot" | "apply" | "rollback",
    options?: { method?: string; verifyTarget?: string; connection?: string },
  ) => {
    if (!nodeId) {
      setNodeNetworkActionError("Node is unavailable for network actions.");
      return;
    }

    setNodeNetworkActionRunning(true);
    setNodeNetworkActionError(null);
    setNodeNetworkActionMessage(null);
    try {
      const body: Record<string, string> = {};
      const method = options?.method?.trim();
      const verifyTarget = options?.verifyTarget?.trim();
      const connection = options?.connection?.trim();
      if (method) body.method = method;
      if (verifyTarget) body.verify_target = verifyTarget;
      if (connection) body.connection = connection;

      const response = await fetch(`/api/network/${encodeURIComponent(nodeId)}/${action}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      const payload = (await response.json().catch(() => ({}))) as {
        ok?: boolean;
        error?: string;
        output?: string;
        rollback_attempted?: boolean;
        rollback_succeeded?: boolean;
        rollback_output?: string;
        rollback_reference?: string;
      };
      if (!response.ok) {
        throw new Error(payload.error || `network ${action} failed (${response.status})`);
      }

      const lines: string[] = [];
      lines.push(action === "snapshot" ? "Network snapshot saved. Edit the Netplan files, then apply." : action === "apply" ? "Network apply completed." : "Network rollback completed.");
      if (payload.rollback_reference) {
        lines.push(`Rollback snapshot: ${payload.rollback_reference}`);
      }
      if (payload.output?.trim()) {
        lines.push(payload.output.trim());
      }
      if (payload.rollback_attempted) {
        lines.push(payload.rollback_succeeded ? "Rollback succeeded." : "Rollback attempted but failed.");
      }
      if (payload.rollback_output?.trim()) {
        lines.push(payload.rollback_output.trim());
      }
      setNodeNetworkActionMessage(lines.join("\n"));
    } catch (err) {
      setNodeNetworkActionError(err instanceof Error ? err.message : `failed to ${action} network`);
    } finally {
      setNodeNetworkActionRunning(false);
    }
  }, [nodeId]);

  return {
    actionRuns,
    actionsLoading,
    expandedActionId,
    setExpandedActionId,
    nodeActionRunning,
    nodeActionMessage,
    nodeActionError,
    nodeNetworkActionRunning,
    nodeNetworkActionMessage,
    nodeNetworkActionError,
    runNodePower,
    runNodeNetworkAction,
  };
}

"use client";

import { useCallback, useEffect, useState } from "react";
import type { ActionRun } from "../../../../console/models";

type UseNodeActionsDataArgs = {
  activeTab: string;
  nodeId: string;
};

export function useNodeActionsData({ activeTab, nodeId }: UseNodeActionsDataArgs) {
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

  const runNodeQuickAction = useCallback(async (command: string, actionLabel?: string) => {
    const trimmed = command.trim();
    if (!nodeId || trimmed === "") {
      setNodeActionError("Command is required.");
      return;
    }

    setNodeActionRunning(true);
    setNodeActionError(null);
    setNodeActionMessage(null);
    try {
      const response = await fetch("/api/actions/execute", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          type: "command",
          actor_id: "owner",
          target: nodeId,
          command: trimmed,
        }),
      });
      const payload = (await response.json()) as { error?: string; run?: { id?: string } };
      if (!response.ok) {
        throw new Error(payload.error || `action failed (${response.status})`);
      }
      const label = (actionLabel ?? trimmed).trim();
      setNodeActionMessage(`Queued ${label}${payload.run?.id ? ` (${payload.run.id.slice(0, 8)})` : ""}`);
    } catch (err) {
      setNodeActionError(err instanceof Error ? err.message : "failed to queue node action");
    } finally {
      setNodeActionRunning(false);
    }
  }, [nodeId]);

  const runNodeNetworkAction = useCallback(async (
    action: "apply" | "rollback",
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
      lines.push(action === "apply" ? "Network apply completed." : "Network rollback completed.");
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
    runNodeQuickAction,
    runNodeNetworkAction,
  };
}

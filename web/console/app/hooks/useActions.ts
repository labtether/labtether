"use client";

import { type FormEvent, useCallback, useEffect, useState } from "react";
import { useSlowStatus, useStatusControls, useStatusSettings } from "../contexts/StatusContext";
import type { ConnectorActionDescriptor } from "../console/models";
import { ensureArray, ensureRecord, ensureString } from "../lib/responseGuards";

export function useActions() {
  const status = useSlowStatus();
  const { fetchStatus, selectedGroupFilter } = useStatusControls();
  const { defaultActorID, defaultActionDryRun } = useStatusSettings();

  const actionRuns = status?.actionRuns ?? [];
  const connectors = status?.connectors ?? [];

  const [selectedConnector, setSelectedConnector] = useState<string>("");
  const [connectorActions, setConnectorActions] = useState<ConnectorActionDescriptor[]>([]);
  const [selectedConnectorAction, setSelectedConnectorAction] = useState<string>("");
  const [actionTarget, setActionTarget] = useState<string>("");
  const [actionParams, setActionParams] = useState<string>("");
  const [actionDryRun, setActionDryRun] = useState<boolean>(defaultActionDryRun);
  const [actionSubmitting, setActionSubmitting] = useState(false);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [connectorActionsError, setConnectorActionsError] = useState<string | null>(null);

  // Auto-select first connector
  useEffect(() => {
    const connectorList = status?.connectors ?? [];
    if (selectedConnector) return;
    if (connectorList.length === 0) return;
    setSelectedConnector(connectorList[0].id);
  }, [status?.connectors, selectedConnector]);

  // Load actions for selected connector
  useEffect(() => {
    if (!selectedConnector) {
      setConnectorActions([]);
      setSelectedConnectorAction("");
      return;
    }

    const controller = new AbortController();
    const load = async () => {
      setConnectorActionsError(null);
      try {
        const response = await fetch(`/api/connectors/${encodeURIComponent(selectedConnector)}/actions`, {
          cache: "no-store",
          signal: controller.signal
        });
        const payload = ensureRecord(await response.json().catch(() => null));
        if (!response.ok) {
          throw new Error(ensureString(payload?.error) || `connector actions fetch failed: ${response.status}`);
        }
        if (controller.signal.aborted) return;
        const actions = ensureArray<ConnectorActionDescriptor>(payload?.actions);
        setConnectorActions(actions);
        if (actions.length > 0) {
          setSelectedConnectorAction((current) => {
            if (current && actions.some((item) => item.id === current)) return current;
            return actions[0].id;
          });
        } else {
          setSelectedConnectorAction("");
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (controller.signal.aborted) return;
        setConnectorActions([]);
        setSelectedConnectorAction("");
        setConnectorActionsError(err instanceof Error ? err.message : "connector actions unavailable");
      }
    };

    void load();
    return () => {
      controller.abort();
    };
  }, [selectedConnector]);

  const selectedActionDescriptor =
    connectorActions.find((action) => action.id === selectedConnectorAction) ?? null;

  const submitConnectorAction = useCallback(async (event: FormEvent) => {
    event.preventDefault();
    if (!selectedConnector || !selectedConnectorAction) {
      setActionMessage("Choose a connector action first.");
      return;
    }

    const parsedParams: Record<string, string> = {};
    const chunks = actionParams
      .split(",")
      .map((chunk) => chunk.trim())
      .filter((chunk) => chunk.length > 0);
    for (const chunk of chunks) {
      const separator = chunk.indexOf("=");
      if (separator <= 0) continue;
      const key = chunk.slice(0, separator).trim();
      const value = chunk.slice(separator + 1).trim();
      if (key !== "") {
        parsedParams[key] = value;
      }
    }
    if (selectedGroupFilter !== "all" && !parsedParams.group_id) {
      parsedParams.group_id = selectedGroupFilter;
    }

    setActionSubmitting(true);
    setActionMessage(null);
    try {
      const payload: Record<string, unknown> = {
        actor_id: defaultActorID,
        type: "connector_action",
        connector_id: selectedConnector,
        action_id: selectedConnectorAction,
        dry_run: actionDryRun
      };
      if (actionTarget.trim() !== "") {
        payload.target = actionTarget.trim();
      }
      if (Object.keys(parsedParams).length > 0) {
        payload.params = parsedParams;
      }

      const response = await fetch("/api/actions/execute", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
      });
      const result = ensureRecord(await response.json().catch(() => null));
      if (!response.ok) {
        throw new Error(ensureString(result?.error) || `action queue failed: ${response.status}`);
      }
      setActionMessage(`Action queued as ${ensureString(result?.job_id) || "job"}`);
      setActionParams("");
      await fetchStatus();
    } catch (err) {
      setActionMessage(err instanceof Error ? err.message : "failed to queue action");
    } finally {
      setActionSubmitting(false);
    }
  }, [selectedConnector, selectedConnectorAction, actionParams, selectedGroupFilter, defaultActorID, actionDryRun, actionTarget, fetchStatus]);

  return {
    connectors,
    selectedConnector,
    setSelectedConnector,
    connectorActions,
    selectedConnectorAction,
    setSelectedConnectorAction,
    selectedActionDescriptor,
    actionTarget,
    setActionTarget,
    actionParams,
    setActionParams,
    actionDryRun,
    setActionDryRun,
    actionSubmitting,
    actionMessage,
    connectorActionsError,
    actionRuns,
    submitConnectorAction
  };
}

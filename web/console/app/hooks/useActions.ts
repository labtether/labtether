"use client";

import { type FormEvent, useCallback, useEffect, useState } from "react";
import { useSlowStatus, useStatusControls, useStatusSettings } from "../contexts/StatusContext";
import type { ConnectorActionDescriptor } from "../console/models";
import { buildConnectorActionRequest, syncActionParamValues, type ActionParamValues } from "../lib/actionForm";
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
  const [actionParamValues, setActionParamValues] = useState<ActionParamValues>({});
  const [actionDryRun, setActionDryRun] = useState<boolean>(defaultActionDryRun);
  const [actionSubmitting, setActionSubmitting] = useState(false);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [connectorActionsError, setConnectorActionsError] = useState<string | null>(null);

  useEffect(() => {
    const connectorList = status?.connectors ?? [];
    if (connectorList.length === 0) {
      if (selectedConnector) {
        setSelectedConnector("");
      }
      return;
    }
    if (selectedConnector && connectorList.some((connector) => connector.id === selectedConnector)) return;
    setSelectedConnector(connectorList[0].id);
  }, [status?.connectors, selectedConnector]);

  useEffect(() => {
    if (!selectedConnector) {
      setConnectorActions([]);
      setSelectedConnectorAction("");
      return;
    }

    const controller = new AbortController();
    const load = async () => {
      setConnectorActionsError(null);
      setConnectorActions([]);
      setSelectedConnectorAction("");
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
  const actionParameters = selectedActionDescriptor?.parameters ?? [];
  const actionRequiresTarget = selectedActionDescriptor?.requires_target ?? false;
  const actionSupportsDryRun = selectedActionDescriptor?.supports_dry_run ?? false;

  useEffect(() => {
    setActionParamValues((current) => syncActionParamValues(current, selectedActionDescriptor));
    setActionMessage(null);
  }, [selectedActionDescriptor]);

  useEffect(() => {
    if (!actionSupportsDryRun && actionDryRun) {
      setActionDryRun(false);
    }
  }, [actionSupportsDryRun, actionDryRun]);

  const setActionParamValue = useCallback((key: string, value: string) => {
    setActionParamValues((current) => ({
      ...current,
      [key]: value,
    }));
  }, []);

  const submitConnectorAction = useCallback(async (event: FormEvent) => {
    event.preventDefault();
    const request = buildConnectorActionRequest({
      actionDryRun,
      actionId: selectedConnectorAction,
      actorId: defaultActorID,
      connectorId: selectedConnector,
      descriptor: selectedActionDescriptor,
      paramValues: actionParamValues,
      selectedGroupFilter,
      target: actionTarget,
    });
    if (!request.ok) {
      setActionMessage(request.error);
      return;
    }

    setActionSubmitting(true);
    setActionMessage(null);
    try {
      const response = await fetch("/api/actions/execute", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(request.payload)
      });
      const result = ensureRecord(await response.json().catch(() => null));
      if (!response.ok) {
        throw new Error(ensureString(result?.error) || `action queue failed: ${response.status}`);
      }
      setActionMessage(`Action queued as ${ensureString(result?.job_id) || "job"}`);
      setActionParamValues((current) => syncActionParamValues(current, selectedActionDescriptor));
      await fetchStatus();
    } catch (err) {
      setActionMessage(err instanceof Error ? err.message : "failed to queue action");
    } finally {
      setActionSubmitting(false);
    }
  }, [
    actionDryRun,
    actionParamValues,
    actionTarget,
    defaultActorID,
    fetchStatus,
    selectedActionDescriptor,
    selectedConnector,
    selectedConnectorAction,
    selectedGroupFilter,
  ]);

  return {
    actionParameters,
    actionParamValues,
    connectors,
    selectedConnector,
    setSelectedConnector,
    connectorActions,
    selectedConnectorAction,
    setSelectedConnectorAction,
    selectedActionDescriptor,
    actionRequiresTarget,
    actionSupportsDryRun,
    actionTarget,
    setActionTarget,
    setActionParamValue,
    actionDryRun,
    setActionDryRun,
    actionSubmitting,
    actionMessage,
    connectorActionsError,
    actionRuns,
    submitConnectorAction
  };
}

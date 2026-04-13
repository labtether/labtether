import type { ConnectorActionDescriptor } from "../console/models";

export type ActionParamValues = Record<string, string>;

export type BuildConnectorActionRequestArgs = {
  actionDryRun: boolean;
  actionId: string;
  actorId: string;
  connectorId: string;
  descriptor: ConnectorActionDescriptor | null;
  paramValues: ActionParamValues;
  selectedGroupFilter: string;
  target: string;
};

export type BuildConnectorActionRequestResult =
  | { ok: true; payload: Record<string, unknown> }
  | { error: string; ok: false };

export function syncActionParamValues(
  current: ActionParamValues,
  descriptor: ConnectorActionDescriptor | null,
): ActionParamValues {
  const next: ActionParamValues = {};
  for (const parameter of descriptor?.parameters ?? []) {
    next[parameter.key] = current[parameter.key] ?? "";
  }
  return next;
}

export function buildConnectorActionRequest({
  actionDryRun,
  actionId,
  actorId,
  connectorId,
  descriptor,
  paramValues,
  selectedGroupFilter,
  target,
}: BuildConnectorActionRequestArgs): BuildConnectorActionRequestResult {
  const trimmedConnectorID = connectorId.trim();
  const trimmedActionID = actionId.trim();
  if (!trimmedConnectorID || !trimmedActionID || !descriptor) {
    return { ok: false, error: "Choose a connector action first." };
  }

  const trimmedTarget = target.trim();
  if (descriptor?.requires_target && trimmedTarget === "") {
    return { ok: false, error: "This action requires a target." };
  }

  const params: Record<string, string> = {};
  for (const parameter of descriptor?.parameters ?? []) {
    const value = (paramValues[parameter.key] ?? "").trim();
    if (parameter.required && value === "") {
      return { ok: false, error: `${parameter.label || parameter.key} is required.` };
    }
    if (value !== "") {
      params[parameter.key] = value;
    }
  }
  if (selectedGroupFilter !== "all" && !params.group_id) {
    params.group_id = selectedGroupFilter;
  }

  const payload: Record<string, unknown> = {
    actor_id: actorId,
    type: "connector_action",
    connector_id: trimmedConnectorID,
    action_id: trimmedActionID,
  };
  if (trimmedTarget !== "") {
    payload.target = trimmedTarget;
  }
  if (Object.keys(params).length > 0) {
    payload.params = params;
  }
  if (descriptor?.supports_dry_run) {
    payload.dry_run = actionDryRun;
  }

  return { ok: true, payload };
}

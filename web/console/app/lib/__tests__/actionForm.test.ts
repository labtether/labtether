import { describe, expect, it } from "vitest";

import type { ConnectorActionDescriptor } from "../../console/models";
import { buildConnectorActionRequest, syncActionParamValues } from "../actionForm";

const restartDescriptor: ConnectorActionDescriptor = {
  id: "container.restart",
  name: "Restart Container",
  requires_target: true,
  supports_dry_run: false,
  parameters: [{ key: "timeout", label: "Timeout", required: false }],
};

describe("syncActionParamValues", () => {
  it("keeps only keys declared by the selected action", () => {
    expect(
      syncActionParamValues(
        { timeout: "15", image: "nginx:latest" },
        restartDescriptor,
      ),
    ).toEqual({ timeout: "15" });
  });
});

describe("buildConnectorActionRequest", () => {
  it("rejects actions that require a target when none is provided", () => {
    expect(
      buildConnectorActionRequest({
        actionDryRun: true,
        actionId: restartDescriptor.id,
        actorId: "owner",
        connectorId: "docker",
        descriptor: restartDescriptor,
        paramValues: {},
        selectedGroupFilter: "all",
        target: "   ",
      }),
    ).toEqual({ ok: false, error: "This action requires a target." });
  });

  it("rejects missing required parameters", () => {
    const descriptor: ConnectorActionDescriptor = {
      id: "image.pull",
      name: "Pull Image",
      requires_target: false,
      supports_dry_run: true,
      parameters: [{ key: "image", label: "Image", required: true }],
    };

    expect(
      buildConnectorActionRequest({
        actionDryRun: true,
        actionId: descriptor.id,
        actorId: "owner",
        connectorId: "docker",
        descriptor,
        paramValues: { image: "  " },
        selectedGroupFilter: "all",
        target: "",
      }),
    ).toEqual({ ok: false, error: "Image is required." });
  });

  it("builds a trimmed payload from descriptor-backed params", () => {
    const result = buildConnectorActionRequest({
      actionDryRun: true,
      actionId: restartDescriptor.id,
      actorId: "owner",
      connectorId: "docker",
      descriptor: restartDescriptor,
      paramValues: { timeout: " 15 ", ignored: "x" },
      selectedGroupFilter: "group-7",
      target: " container-1 ",
    });

    expect(result).toEqual({
      ok: true,
      payload: {
        actor_id: "owner",
        type: "connector_action",
        connector_id: "docker",
        action_id: "container.restart",
        target: "container-1",
        params: {
          timeout: "15",
          group_id: "group-7",
        },
      },
    });
  });

  it("omits dry_run when the action does not support it", () => {
    const result = buildConnectorActionRequest({
      actionDryRun: true,
      actionId: restartDescriptor.id,
      actorId: "owner",
      connectorId: "docker",
      descriptor: restartDescriptor,
      paramValues: {},
      selectedGroupFilter: "all",
      target: "container-1",
    });

    expect(result).toEqual({
      ok: true,
      payload: {
        actor_id: "owner",
        type: "connector_action",
        connector_id: "docker",
        action_id: "container.restart",
        target: "container-1",
      },
    });
  });
});

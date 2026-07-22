import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../hooks/useTailscaleServeStatus", () => ({
  useTailscaleServeStatus: () => ({
    status: {
      tailscale_installed: false,
      logged_in: false,
      serve_status: "not_installed",
      serve_configured: false,
      suggested_target: "http://127.0.0.1:3000",
      recommendation_state: "disabled_by_choice",
      recommendation_message: "Disabled by choice.",
      can_manage: false,
      management_mode: "guided",
      desired_mode: "off",
      desired_mode_source: "ui",
      desired_target_source: "default",
    },
    loading: false,
    error: "",
    refresh: vi.fn(),
  }),
}));

import { runtimeSettingKeys, type RuntimeSettingEntry } from "../../../../../console/models";
import { TailscaleServeCard } from "../TailscaleServeCard";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

function runtimeSetting(key: string, value: string): RuntimeSettingEntry {
  return {
    key,
    label: key,
    description: key,
    scope: "hub",
    type: "string",
    env_var: "",
    default_value: value,
    effective_value: value,
    source: "ui",
    sensitive: false,
    configured: true,
  };
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

describe("TailscaleServeCard accessibility", () => {
  it("names its mode and upstream controls with their visible labels", async () => {
    const mode = runtimeSetting(runtimeSettingKeys.remoteAccessMode, "off");
    const target = runtimeSetting(runtimeSettingKeys.remoteAccessTailscaleServeTarget, "");
    await act(async () => {
      root.render(
        <TailscaleServeCard
          copyToClipboard={vi.fn()}
          copied=""
          runtimeSettings={[mode, target]}
          runtimeDraftValues={{ [mode.key]: "off", [target.key]: "" }}
          setRuntimeDraftValues={vi.fn()}
          runtimeSettingsLoading={false}
          runtimeSettingsSaving={false}
          runtimeSettingsMessage={null}
          saveRuntimeSettings={vi.fn()}
          resetRuntimeSetting={vi.fn()}
          canManageActions={false}
        />,
      );
    });

    expect(container.querySelector('select[aria-label="Preferred Access Mode"]')).not.toBeNull();
    expect(container.querySelector('input[aria-label="Preferred Serve Upstream"]')).not.toBeNull();
  });
});

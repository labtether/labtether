import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../contexts/ToastContext", () => ({
  useToast: () => ({ addToast: vi.fn() }),
}));
vi.mock("../../../contexts/StatusContext", () => ({
  useStatusControls: () => ({ fetchStatus: vi.fn() }),
}));
vi.mock("../../../hooks/useEnrollment", () => ({
  useEnrollment: () => ({
    hubURL: "https://hub.example.test:8443",
    hubCandidates: [{
      kind: "external",
      label: "Configured external URL",
      host: "hub.example.test:8443",
      hub_url: "https://hub.example.test:8443",
      ws_url: "wss://hub.example.test:8443/ws/agent",
    }],
    selectHubURL: vi.fn(),
  }),
}));
vi.mock("../../../hooks/useHomeAssistantSettings", () => ({
  useHomeAssistantSettings: () => ({
    baseURL: "http://homeassistant.example.test:8123",
    setBaseURL: vi.fn(),
    token: "",
    setToken: vi.fn(),
    displayName: "Home Assistant",
    setDisplayName: vi.fn(),
    skipVerify: false,
    setSkipVerify: vi.fn(),
    intervalSeconds: 60,
    setIntervalSeconds: vi.fn(),
    collectorID: "",
    credentialID: "",
    configured: false,
    loading: false,
    saving: false,
    testing: false,
    running: false,
    error: "Connector load failed.",
    message: "",
    save: vi.fn(),
    testConnection: vi.fn(),
    runNow: vi.fn(),
  }),
}));

import { HomeAssistantSetupStep } from "../HomeAssistantSetupStep";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

describe("HomeAssistantSetupStep", () => {
  it("shows the LabTether hub address separately from the Home Assistant connector address", async () => {
    await act(async () => {
      root.render(
        <HomeAssistantSetupStep
          onBack={vi.fn()}
          onClose={vi.fn()}
          setupMode="advanced"
        />,
      );
    });

    const copiedValues = [...container.querySelectorAll("code")].map((node) => node.textContent);
    expect(copiedValues).toContain("https://hub.example.test:8443");
    expect(copiedValues).not.toContain("http://homeassistant.example.test:8123");

    expect(container.querySelector('label[for="homeassistant-base-url"]')?.textContent).toBe("Home Assistant Base URL");
    expect(container.querySelector('#homeassistant-base-url')).not.toBeNull();
    expect(container.querySelector('label[for="homeassistant-access-token"]')).not.toBeNull();
    expect(container.querySelector('label[for="homeassistant-display-name"]')).not.toBeNull();
    expect(container.querySelector('label[for="homeassistant-poll-interval"]')).not.toBeNull();
    expect(container.querySelector('[role="alert"]')?.textContent).toBe("Connector load failed.");
  });
});

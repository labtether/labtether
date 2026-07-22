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
    hubURL: "http://localhost",
    hubCandidates: [
      {
        kind: "request",
        label: "Localhost",
        host: "localhost",
        hub_url: "http://localhost",
        ws_url: "ws://localhost/ws/agent",
      },
      {
        kind: "external",
        label: "Configured external URL",
        host: "hub.example.test:8443",
        hub_url: "https://hub.example.test:8443",
        ws_url: "wss://hub.example.test:8443/ws/agent",
      },
      {
        kind: "lan",
        label: "LAN",
        host: "192.168.48.3:8443",
        hub_url: "https://192.168.48.3:8443",
        ws_url: "wss://192.168.48.3:8443/ws/agent",
      },
    ],
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

import {
  HomeAssistantSetupStep,
  homeAssistantHubHTTPWarning,
  homeAssistantHubCandidates,
  preferredHomeAssistantHubURL,
  validateHomeAssistantHubOrigin,
} from "../HomeAssistantSetupStep";

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
    expect(copiedValues).not.toContain("http://localhost");
    expect(copiedValues).not.toContain("https://192.168.48.3:8443");
    expect(copiedValues).not.toContain("http://homeassistant.example.test:8123");

    const hubURLInput = container.querySelector<HTMLInputElement>('#homeassistant-hub-url');
    expect(hubURLInput?.value).toBe("https://hub.example.test:8443");
    const suggestedHubURLs = [...container.querySelectorAll<HTMLOptionElement>('#homeassistant-hub-candidates option')]
      .map((option) => option.value);
    expect(suggestedHubURLs).toEqual([
      "https://hub.example.test:8443",
    ]);
    expect(container.textContent).toContain("Use the direct Hub address, not this web console address.");

    expect(container.querySelector('label[for="homeassistant-base-url"]')?.textContent).toBe("Home Assistant Base URL");
    expect(container.querySelector('#homeassistant-base-url')).not.toBeNull();
    expect(container.querySelector('label[for="homeassistant-access-token"]')).not.toBeNull();
    expect(container.querySelector('label[for="homeassistant-display-name"]')).not.toBeNull();
    expect(container.querySelector('label[for="homeassistant-poll-interval"]')).not.toBeNull();
    expect(container.querySelector('[role="alert"]')?.textContent).toBe("Connector load failed.");
  });

  it("never auto-selects the console origin or an unverified LAN suggestion", () => {
    const candidates = [
      {
        kind: "request",
        label: "Localhost",
        host: "console.example.test",
        hub_url: "https://console.example.test",
        ws_url: "wss://console.example.test/ws/agent",
      },
      {
        kind: "lan",
        label: "LAN",
        host: "192.168.48.3:8443",
        hub_url: "https://192.168.48.3:8443",
        ws_url: "wss://192.168.48.3:8443/ws/agent",
      },
    ];

    expect(homeAssistantHubCandidates(candidates, "https://console.example.test")).toEqual([]);
    expect(preferredHomeAssistantHubURL(
      candidates,
      "https://192.168.48.3:8443",
      "https://console.example.test",
    )).toBe("");
  });

  it("matches Home Assistant's credential-bearing origin validation", () => {
    expect(validateHomeAssistantHubOrigin("https://hub.example.test:8443")).toBe("");
    expect(validateHomeAssistantHubOrigin("http://127.0.0.1:8080")).toBe("");
    expect(validateHomeAssistantHubOrigin("https://user:secret@hub.example.test")).not.toBe("");
    expect(validateHomeAssistantHubOrigin("https://hub.example.test/api?token=secret#fragment")).not.toBe("");
    expect(homeAssistantHubHTTPWarning("http://hub.example.test:8080")).toContain("Allow insecure HTTP");
    expect(homeAssistantHubHTTPWarning("http://localhost:8080")).toBe("");
  });
});

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  deleteEnrollmentToken: vi.fn(async () => undefined),
  generateToken: vi.fn(async () => "etok_test"),
  clearNewToken: vi.fn(),
  newRawToken: "one-time-token",
  newTokenID: "etok_test",
}));

vi.mock("../../../hooks/useEnrollment", () => ({
  deleteEnrollmentToken: mocks.deleteEnrollmentToken,
  useEnrollment: () => ({
    hubURL: "https://hub.example.test:8443",
    wsURL: "wss://hub.example.test:8443/ws/agent",
    hubCandidates: [],
    enrollmentTokens: [],
    selectHubURL: vi.fn(),
    newRawToken: mocks.newRawToken,
    newTokenID: mocks.newTokenID,
    generating: false,
    generateToken: mocks.generateToken,
    clearNewToken: mocks.clearNewToken,
    error: "",
  }),
}));
vi.mock("../../../contexts/StatusContext", () => ({
  useFastStatus: () => ({ assets: [] }),
}));
vi.mock("../../../contexts/ToastContext", () => ({
  useToast: () => ({ addToast: vi.fn() }),
}));

import { AgentSetupStep } from "../AgentSetupStep";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  mocks.deleteEnrollmentToken.mockClear();
  mocks.generateToken.mockClear();
  mocks.clearNewToken.mockClear();
  mocks.newRawToken = "one-time-token";
  mocks.newTokenID = "etok_test";
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText: vi.fn(async () => undefined) },
  });
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(() => {
  container.remove();
});

async function mountStep() {
  await act(async () => {
    root.render(
      <AgentSetupStep onBack={vi.fn()} onClose={vi.fn()} />,
    );
  });
}

async function unmountStep() {
  await act(async () => {
    root.unmount();
    await Promise.resolve();
  });
}

describe("AgentSetupStep token lifecycle", () => {
  it("does not mint until an expected hostname is supplied, then requests an asset-bound token", async () => {
    mocks.newRawToken = "";
    mocks.newTokenID = "";
    await mountStep();

    expect(mocks.generateToken).not.toHaveBeenCalled();
    const hostname = container.querySelector<HTMLInputElement>('input[aria-label="Expected hostname"]');
    const createButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.trim() === "Create one-time token",
    );
    expect(hostname).not.toBeNull();
    expect(createButton?.disabled).toBe(true);
    await act(async () => {
      if (hostname) {
        const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
        setter?.call(hostname, "Server 01");
        hostname.dispatchEvent(new Event("input", { bubbles: true }));
      }
    });
    expect(createButton?.disabled).toBe(false);
    await act(async () => {
      createButton?.click();
      await Promise.resolve();
    });
    expect(mocks.generateToken).toHaveBeenCalledWith("add-device-wizard", 24, 1, {
      scope: "asset",
      assetID: "Server 01",
    });

    await unmountStep();
  });

  it("revokes an untouched generated token when the user exits", async () => {
    await mountStep();
    await unmountStep();

    expect(mocks.deleteEnrollmentToken).toHaveBeenCalledWith("etok_test", {
      keepalive: true,
    });
    expect(mocks.clearNewToken).toHaveBeenCalled();
  });

  it("retains a token the user deliberately copied", async () => {
    await mountStep();
    const copyToken = container.querySelector<HTMLButtonElement>(
      'button[aria-label="Copy enrollment token"]',
    );
    expect(copyToken).not.toBeNull();
    await act(async () => {
      copyToken?.click();
      await Promise.resolve();
    });
    await unmountStep();

    expect(mocks.deleteEnrollmentToken).not.toHaveBeenCalled();
  });

  it("gives copy and target controls distinct accessible names", async () => {
    await mountStep();

    expect(container.querySelector('[aria-label="Copy enrollment token"]')).not.toBeNull();
    expect(container.querySelector('[aria-label="Copy Hub URL"]')).not.toBeNull();
    expect(container.querySelector('[aria-label="Copy WebSocket URL"]')).not.toBeNull();
    expect(container.querySelector('[aria-label="Copy Linux installer command"]')).not.toBeNull();

    await unmountStep();
  });
});

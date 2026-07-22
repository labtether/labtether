import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  deleteEnrollmentToken: vi.fn(async () => undefined),
  generateToken: vi.fn(async () => "etok_test"),
  clearNewToken: vi.fn(),
  newTokenID: "",
}));

vi.mock("../../../hooks/useEnrollment", () => ({
  deleteEnrollmentToken: mocks.deleteEnrollmentToken,
  useEnrollment: () => ({
    hubURL: "https://hub.example.test:8443",
    wsURL: "wss://hub.example.test:8443/ws/agent",
    hubCandidates: [],
    enrollmentTokens: [],
    selectHubURL: vi.fn(),
    newRawToken: "one-time-token",
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
  mocks.newTokenID = "";
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

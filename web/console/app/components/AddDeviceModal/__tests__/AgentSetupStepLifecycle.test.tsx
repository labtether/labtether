import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import auth from "../../../../messages/en/auth.json";

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
      <NextIntlClientProvider locale="en" messages={{ auth }}>
        <AgentSetupStep onBack={vi.fn()} onClose={vi.fn()} />
      </NextIntlClientProvider>,
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
    expect(container.querySelector('[aria-label="Copy Linux install command"]')).not.toBeNull();

    await unmountStep();
  });

  // Risk: the person compares this value by eye with the one Mac or Windows setup shows before trusting the Hub.
  // A different grouping, a missing value, or a value shown for a Hub with no CA of its own breaks that check.
  describe("Hub certificate fingerprint", () => {
    const fingerprint = "3f9a1c0e5b7d2468ace013579bdf02468ace13579bdf0a1b2c3d4e5f60718293";
    const grouped = "3f9a 1c0e 5b7d 2468 ace0 1357 9bdf 0246 8ace 1357 9bdf 0a1b 2c3d 4e5f 6071 8293";
    const tlsInfo = (body: unknown) =>
      vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } })));
    const pick = async (name: string) => {
      const button = [...container.querySelectorAll("button")].find((b) => b.textContent === name);
      await act(async () => button?.click());
    };

    afterEach(() => vi.unstubAllGlobals());

    it("shows the Hub CA's fingerprint in groups of four for macOS and Windows, and copies the plain value", async () => {
      tlsInfo({ ca_available: true, ca_fingerprint_sha256: fingerprint.toUpperCase() });
      await mountStep();
      expect(container.textContent).not.toContain(grouped);

      for (const platform of ["macOS", "Windows"]) {
        await pick(platform);
        expect(container.textContent).toContain("Hub certificate fingerprint");
        expect(container.textContent).toContain(grouped);
      }
      await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="Copy Hub certificate fingerprint"]')?.click());
      expect(navigator.clipboard.writeText).toHaveBeenCalledWith(fingerprint);

      await unmountStep();
    });

    it("shows no fingerprint when the Hub has no CA of its own", async () => {
      tlsInfo({ ca_available: false, ca_fingerprint_sha256: fingerprint });
      await mountStep();
      await pick("macOS");
      expect(container.textContent).not.toContain("Hub certificate fingerprint");

      await unmountStep();
    });
  });
});

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SSHHubKeyCard } from "../SSHHubKeyCard";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
let fetchMock: ReturnType<typeof vi.fn>;
let clipboardWrite: ReturnType<typeof vi.fn>;

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  fetchMock = vi.fn();
  clipboardWrite = vi.fn(async () => undefined);
  vi.stubGlobal("fetch", fetchMock);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText: clipboardWrite },
  });
  if (!("PointerEvent" in window)) {
    vi.stubGlobal("PointerEvent", MouseEvent);
  }
  if (!("hasPointerCapture" in HTMLElement.prototype)) {
    Object.defineProperty(HTMLElement.prototype, "hasPointerCapture", {
      configurable: true,
      value: () => false,
    });
  }
  if (!("scrollIntoView" in HTMLElement.prototype)) {
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      configurable: true,
      value: () => undefined,
    });
  }
});

afterEach(async () => {
  await act(async () => {
    root.unmount();
  });
  container.remove();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

async function settle() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

async function renderCard() {
  await act(async () => {
    root.render(<SSHHubKeyCard />);
  });
  await settle();
}

function button(text: string): HTMLButtonElement {
  const matches = [...document.body.querySelectorAll<HTMLButtonElement>("button")].filter(
    (candidate) => candidate.textContent?.trim().includes(text),
  );
  const match = matches.at(-1);
  if (!match) throw new Error(`missing button ${text}`);
  return match;
}

async function click(element: HTMLElement) {
  await act(async () => {
    element.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    await Promise.resolve();
  });
  await settle();
}

async function setValue(element: HTMLInputElement | HTMLSelectElement, value: string) {
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(element), "value")?.set;
    if (!setter) throw new Error("element value setter unavailable");
    setter.call(element, value);
    element.dispatchEvent(new Event(element instanceof HTMLSelectElement ? "change" : "input", { bubbles: true }));
  });
}

function initialKeyResponse() {
  return new Response(JSON.stringify({
    public_key: "ssh-ed25519 AAAAold labtether-hub",
    key_type: "ed25519",
    fingerprint_sha256: "SHA256:old",
  }), { status: 200, headers: { "content-type": "application/json" } });
}

describe("SSH hub key settings card", () => {
  it("views, copies, and rotates only after exact typed confirmation", async () => {
    fetchMock
      .mockResolvedValueOnce(initialKeyResponse())
      .mockResolvedValueOnce(new Response(JSON.stringify({
        status: "rotated",
        public_key: "ssh-rsa AAAAnew labtether-hub",
        key_type: "rsa",
        fingerprint_sha256: "SHA256:new",
        agents_updated: 2,
        agents_total: 2,
        old_key_removal_failures: 0,
      }), { status: 200, headers: { "content-type": "application/json" } }));

    await renderCard();
    expect(document.body.textContent).toContain("SHA256:old");
    expect(document.body.textContent).toContain("ssh-ed25519 AAAAold labtether-hub");

    await click(button("Copy public key"));
    expect(clipboardWrite).toHaveBeenCalledWith("ssh-ed25519 AAAAold labtether-hub");
    expect(document.body.textContent).toContain("Public key copied to clipboard");

    await click(button("Rotate key"));
    expect(document.body.querySelector('[role="dialog"]')).toBeTruthy();
    expect(button("Rotate key").disabled).toBe(true);

    const keyType = document.body.querySelector<HTMLSelectElement>("#ssh-hub-key-type");
    const reason = document.body.querySelector<HTMLInputElement>("#ssh-hub-key-reason");
    const confirmation = document.body.querySelector<HTMLInputElement>("#ssh-hub-key-confirmation");
    if (!keyType || !reason || !confirmation) throw new Error("missing rotation fields");
    await setValue(keyType, "rsa");
    await setValue(reason, " LTQA-245 proof ");
    await setValue(confirmation, "ROTATE");
    expect(button("Rotate key").disabled).toBe(false);

    await click(button("Rotate key"));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    const request = fetchMock.mock.calls[1]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toEqual({
      key_type: "rsa",
      reason: "LTQA-245 proof",
      confirm: "ROTATE",
    });
    expect(document.body.textContent).toContain("SHA256:new");
    expect(document.body.textContent).toContain("2 of 2 connected agents were updated");
  });

  it("keeps the existing key visible and the dialog open on an honest backend failure", async () => {
    fetchMock
      .mockResolvedValueOnce(initialKeyResponse())
      .mockResolvedValueOnce(new Response(JSON.stringify({
        error: "failed to stage the new hub SSH key on all connected agents; the existing key remains active",
      }), { status: 502, headers: { "content-type": "application/json" } }));

    await renderCard();
    await click(button("Rotate key"));
    const confirmation = document.body.querySelector<HTMLInputElement>("#ssh-hub-key-confirmation");
    if (!confirmation) throw new Error("missing confirmation field");
    await setValue(confirmation, "ROTATE");
    await click(button("Rotate key"));

    expect(document.body.querySelector('[role="dialog"]')).toBeTruthy();
    expect(document.body.textContent).toContain("existing key remains active");
    expect(document.body.textContent).toContain("SHA256:old");
    expect(document.body.textContent).not.toContain("SHA256:new");
  });
});

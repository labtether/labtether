import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CredentialProfilesCard } from "../CredentialProfilesCard";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
  if (!("PointerEvent" in window)) vi.stubGlobal("PointerEvent", MouseEvent);
  if (!("hasPointerCapture" in HTMLElement.prototype)) {
    Object.defineProperty(HTMLElement.prototype, "hasPointerCapture", { configurable: true, value: () => false });
  }
  if (!("scrollIntoView" in HTMLElement.prototype)) {
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", { configurable: true, value: () => undefined });
  }
});

afterEach(async () => {
  await act(async () => root.unmount());
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
  await act(async () => root.render(<CredentialProfilesCard />));
  await settle();
}

function button(text: string): HTMLButtonElement {
  const match = [...document.body.querySelectorAll<HTMLButtonElement>("button")]
    .filter((candidate) => candidate.textContent?.includes(text))
    .at(-1);
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

async function setValue(element: HTMLInputElement | HTMLTextAreaElement, value: string) {
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(element), "value")?.set;
    if (!setter) throw new Error("value setter unavailable");
    setter.call(element, value);
    element.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

const inventoryResponse = () => new Response(JSON.stringify({
  profiles: [{
    id: "cred_ssh_1",
    name: "Lab SSH",
    kind: "ssh_password",
    username: "root",
    status: "active",
    created_at: "2026-07-15T00:00:00Z",
    updated_at: "2026-07-15T00:00:00Z",
  }],
}), { status: 200, headers: { "content-type": "application/json" } });

describe("CredentialProfilesCard", () => {
  it("keeps exact-byte create input and an accessible dialog visible on backend failure", async () => {
    fetchMock
      .mockResolvedValueOnce(inventoryResponse())
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: "credential profile global limit reached" }), { status: 409 }));
    await renderCard();
    await click(button("Create profile"));

    const dialog = document.body.querySelector('[role="dialog"]');
    if (!dialog) throw new Error("missing create dialog");
    const inputs = dialog.querySelectorAll<HTMLInputElement>("input");
    await setValue(inputs[0], "Exact profile");
    await setValue(inputs[3], " secret with edges ");
    await click(button("Create profile"));

    expect(fetchMock).toHaveBeenCalledTimes(2);
    const request = fetchMock.mock.calls[1]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body)).secret).toBe(" secret with edges ");
    expect(document.body.querySelector('[role="dialog"]')).toBeTruthy();
    expect(document.body.querySelector('[role="alert"]')?.textContent).toContain("global limit reached");
  });

  it("keeps delete confirmation open and renders redacted live-reference counts", async () => {
    fetchMock
      .mockResolvedValueOnce(inventoryResponse())
      .mockResolvedValueOnce(new Response(JSON.stringify({
        error: "credential profile is in use",
        references: [{ resource: "asset_protocol_configs", count: 2 }],
      }), { status: 409 }));
    await renderCard();
    await click(button("Delete"));
    await click(button("Delete profile"));

    expect(document.body.querySelector('[role="dialog"]')).toBeTruthy();
    expect(document.body.querySelector('[role="alert"]')?.textContent).toContain("2 asset protocol configurations");
    expect(document.body.textContent).not.toContain("asset-secret-canary");
  });
});

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ApiKeysCard } from "../ApiKeysCard";

const mocks = vi.hoisted(() => ({
  createKey: vi.fn(),
  updateKey: vi.fn(),
  revokeKey: vi.fn(),
  keys: [] as Array<Record<string, unknown>>,
}));

vi.mock("next-intl", () => ({
  useTranslations: () => (key: string) => key,
}));

vi.mock("../../../../../contexts/AuthContext", () => ({
  useAuth: () => ({ user: { id: "admin-1", role: "admin" } }),
}));

vi.mock("../../../../../contexts/StatusContext", () => ({
  useFastStatus: () => ({
    assets: [
      { id: "node-b", name: "Beta" },
      { id: "node-a", name: "Alpha" },
    ],
  }),
}));

vi.mock("../../../../../hooks/useApiKeys", () => ({
  useApiKeys: () => ({
    keys: mocks.keys,
    loading: false,
    error: "",
    createKey: mocks.createKey,
    updateKey: mocks.updateKey,
    revokeKey: mocks.revokeKey,
  }),
}));

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  mocks.createKey.mockReset();
  mocks.updateKey.mockReset();
  mocks.revokeKey.mockReset();
  mocks.keys = [];
  mocks.createKey.mockResolvedValue({
    id: "key-1",
    name: "Scoped integration",
    prefix: "abcd",
    raw_key: "lt_abcd_secret",
    role: "operator",
    scopes: ["*"],
    allowed_assets: ["node-a"],
    created_by: "admin-1",
    created_at: "2026-07-15T00:00:00Z",
  });
  mocks.updateKey.mockResolvedValue(undefined);
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => {
    root.unmount();
  });
  container.remove();
});

async function click(element: HTMLElement): Promise<void> {
  await act(async () => {
    element.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
}

async function setInputValue(element: HTMLInputElement, value: string): Promise<void> {
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
    if (!setter) throw new Error("input value setter is unavailable");
    setter.call(element, value);
    element.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function setSelectValue(element: HTMLSelectElement, value: string): Promise<void> {
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")?.set;
    if (!setter) throw new Error("select value setter is unavailable");
    setter.call(element, value);
    element.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

describe("ApiKeysCard asset restrictions", () => {
  it("requires and submits an explicit asset allow-list for restricted keys", async () => {
    await act(async () => {
      root.render(<ApiKeysCard />);
    });

    const nameInput = container.querySelector<HTMLInputElement>('input[type="text"]');
    if (!nameInput) throw new Error("missing API key name input");
    await setInputValue(nameInput, "Scoped integration");

    const assetModeRadios = container.querySelectorAll<HTMLInputElement>('input[name="api-key-create-assets-mode"]');
    expect(assetModeRadios).toHaveLength(2);
    await click(assetModeRadios[1]!);

    const createButton = [...container.querySelectorAll<HTMLButtonElement>("button")].find((button) =>
      button.textContent?.includes("apiKeys.createKey"),
    );
    if (!createButton) throw new Error("missing create button");
    expect(createButton.disabled).toBe(true);
    expect(container.textContent).toContain("apiKeys.allowedAssetsRequired");

    const alphaLabel = [...container.querySelectorAll<HTMLLabelElement>("label")].find((label) =>
      label.textContent?.includes("node-a"),
    );
    const alphaCheckbox = alphaLabel?.querySelector<HTMLInputElement>('input[type="checkbox"]');
    if (!alphaCheckbox) throw new Error("missing node-a checkbox");
    await click(alphaCheckbox);

    expect(createButton.disabled).toBe(false);
    await click(createButton);

    expect(mocks.createKey).toHaveBeenCalledTimes(1);
    expect(mocks.createKey).toHaveBeenCalledWith(expect.objectContaining({
      name: "Scoped integration",
      allowed_assets: ["node-a"],
    }));
  });

  it("can remove a stored restriction and clear an expiry explicitly", async () => {
    mocks.keys = [{
      id: "key-2",
      name: "Existing integration",
      prefix: "efgh",
      role: "viewer",
      scopes: ["assets:read"],
      allowed_assets: ["node-b"],
      expires_at: "2026-08-15T00:00:00Z",
      created_by: "admin-1",
      created_at: "2026-07-15T00:00:00Z",
    }];
    await act(async () => {
      root.render(<ApiKeysCard />);
    });

    const editButton = container.querySelector<HTMLButtonElement>('button[aria-label="apiKeys.edit"]');
    if (!editButton) throw new Error("missing edit button");
    await click(editButton);

    const dialog = container.querySelector<HTMLElement>('[role="dialog"][aria-labelledby="api-key-edit-title"]');
    if (!dialog) throw new Error("missing edit dialog");
    const modeRadios = dialog.querySelectorAll<HTMLInputElement>('input[type="radio"]');
    expect(modeRadios).toHaveLength(2);
    await click(modeRadios[0]!);

    const expiry = dialog.querySelector<HTMLSelectElement>("#api-key-edit-expiry");
    if (!expiry) throw new Error("missing expiry selector");
    await setSelectValue(expiry, "never");

    const saveButton = [...dialog.querySelectorAll<HTMLButtonElement>("button")].find((button) =>
      button.textContent?.includes("apiKeys.saveChanges"),
    );
    if (!saveButton) throw new Error("missing save button");
    await click(saveButton);

    expect(mocks.updateKey).toHaveBeenCalledWith("key-2", {
      name: "Existing integration",
      scopes: ["assets:read"],
      allowed_assets: [],
      expires_at: null,
    });
  });
});

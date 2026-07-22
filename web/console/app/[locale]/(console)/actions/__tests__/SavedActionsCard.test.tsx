import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Asset } from "../../../../console/models";
import { SavedActionsCard } from "../SavedActionsCard";

vi.mock("next-intl", () => ({
  useTranslations: () => (key: string, values?: Record<string, unknown>) => {
    if (!values) return key;
    return `${key} ${Object.values(values).join(" ")}`;
  },
}));

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const assets: Asset[] = [
  { id: "node-a", name: "Alpha", type: "host", source: "agent", status: "online", last_seen_at: "2026-07-15T00:00:00Z" },
  { id: "node-b", name: "Beta", type: "host", source: "agent", status: "online", last_seen_at: "2026-07-15T00:00:00Z" },
];

let container: HTMLDivElement;
let root: Root;

function envelope(data: unknown, status = 200): Response {
  return new Response(JSON.stringify({ data }), { status, headers: { "Content-Type": "application/json" } });
}

async function settle(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

async function click(element: HTMLElement): Promise<void> {
  await act(async () => {
    element.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  await settle();
}

async function setInput(element: HTMLInputElement, value: string): Promise<void> {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set?.call(element, value);
    element.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function setTextarea(element: HTMLTextAreaElement, value: string): Promise<void> {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set?.call(element, value);
    element.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function setSelect(element: HTMLSelectElement, value: string): Promise<void> {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")?.set?.call(element, value);
    element.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

function button(label: string, scope: ParentNode = document.body): HTMLButtonElement {
  const match = [...scope.querySelectorAll<HTMLButtonElement>("button")].find((item) => item.textContent?.trim().includes(label));
  if (!match) throw new Error(`missing button ${label}`);
  return match;
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("SavedActionsCard", () => {
  it("completes create, reload, view, run-result, and typed-delete journeys", async () => {
    let storedAction: Record<string, unknown> | null = null;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
      const method = (init?.method ?? (input instanceof Request ? input.method : "GET")).toUpperCase();
      if (url === "/api/v2/actions?limit=100" && method === "GET") {
        return new Response(JSON.stringify({ data: storedAction ? [storedAction] : [], meta: { total: storedAction ? 1 : 0 } }), { status: 200 });
      }
      if (url === "/api/v2/actions" && method === "POST") {
        const request = JSON.parse(String(init?.body)) as Record<string, unknown>;
        storedAction = {
          ...request,
          id: "act-1",
          created_by: "owner-1",
          created_at: "2026-07-15T00:00:00Z",
        };
        return envelope(storedAction, 201);
      }
      if (url === "/api/v2/actions/act-1" && method === "GET") return envelope(storedAction);
      if (url === "/api/v2/actions/act-1/run" && method === "POST") {
        return envelope({
          action_id: "act-1",
          steps: [
            { name: "Check Alpha", target: "node-a", exit_code: 0, output: "alpha ok", duration_ms: 12 },
            { name: "Check Beta", target: "node-b", error: "exec_failed", message: "beta failed" },
          ],
        });
      }
      if (url === "/api/v2/actions/act-1" && method === "DELETE") {
        storedAction = null;
        return envelope({ status: "deleted" });
      }
      throw new Error(`unexpected fetch ${method} ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    await act(async () => root.render(<SavedActionsCard assets={assets} />));
    await settle();
    expect(container.textContent).toContain("empty");

    await click(button("reload", container));
    expect(fetchMock.mock.calls.filter((call) => call[0] === "/api/v2/actions?limit=100")).toHaveLength(2);

    await click(button("create", container));
    const createDialog = document.body.querySelector<HTMLElement>('[role="dialog"]');
    if (!createDialog) throw new Error("missing create dialog");
    const inputs = createDialog.querySelectorAll<HTMLInputElement>("input");
    const textareas = createDialog.querySelectorAll<HTMLTextAreaElement>("textarea");
    const selects = createDialog.querySelectorAll<HTMLSelectElement>("select");
    expect(inputs).toHaveLength(2);
    expect(textareas).toHaveLength(2);
    expect(selects).toHaveLength(1);
    await setInput(inputs[0]!, "Fleet health check");
    await setInput(inputs[1]!, "Check Alpha");
    await setTextarea(textareas[0]!, "Checks both lab devices");
    await setTextarea(textareas[1]!, "uptime");
    await setSelect(selects[0]!, "node-a");

    await click(button("addStep", createDialog));
    const updatedInputs = createDialog.querySelectorAll<HTMLInputElement>("input");
    const updatedTextareas = createDialog.querySelectorAll<HTMLTextAreaElement>("textarea");
    const updatedSelects = createDialog.querySelectorAll<HTMLSelectElement>("select");
    await setInput(updatedInputs[2]!, "Check Beta");
    await setTextarea(updatedTextareas[2]!, "hostname");
    await setSelect(updatedSelects[1]!, "node-b");

    const form = createDialog.querySelector("form");
    if (!form) throw new Error("missing create form");
    await act(async () => {
      form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
    });
    await settle();

    const createCall = fetchMock.mock.calls.find((call) => call[0] === "/api/v2/actions" && call[1]?.method === "POST");
    expect(createCall).toBeTruthy();
    expect(JSON.parse(String(createCall?.[1]?.body))).toEqual({
      name: "Fleet health check",
      description: "Checks both lab devices",
      steps: [
        { name: "Check Alpha", command: "uptime", target: "node-a" },
        { name: "Check Beta", command: "hostname", target: "node-b" },
      ],
    });
    expect(container.textContent).toContain("Fleet health check");

    await click(button("view", container));
    expect(fetchMock).toHaveBeenCalledWith("/api/v2/actions/act-1", { cache: "no-store" });
    const detailDialog = document.body.querySelector<HTMLElement>('[role="dialog"]');
    if (!detailDialog) throw new Error("missing details dialog");
    expect(detailDialog.textContent).toContain("uptime");
    expect(detailDialog.textContent).toContain("node-b");

    await click(button("runNow", detailDialog));
    expect(fetchMock).toHaveBeenCalledWith("/api/v2/actions/act-1/run", { method: "POST", cache: "no-store" });
    expect(detailDialog.textContent).toContain("alpha ok");
    expect(detailDialog.textContent).toContain("beta failed");
    expect(detailDialog.textContent).toContain("exitCode 0");

    await act(async () => {
      document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    });
    await settle();

    await click(button("delete", container));
    const deleteDialog = document.body.querySelector<HTMLElement>('[role="dialog"]');
    if (!deleteDialog) throw new Error("missing delete dialog");
    const deleteButton = button("deleteActionButton", deleteDialog);
    expect(deleteButton.disabled).toBe(true);
    const confirmation = deleteDialog.querySelector<HTMLInputElement>("input");
    if (!confirmation) throw new Error("missing delete confirmation input");
    await setInput(confirmation, "Fleet health check");
    expect(deleteButton.disabled).toBe(false);
    await click(deleteButton);
    expect(fetchMock).toHaveBeenCalledWith("/api/v2/actions/act-1", { method: "DELETE", cache: "no-store" });
    expect(container.textContent).not.toContain("Check Alpha");
  });

  it("keeps the create dialog open and reports an upstream failure honestly", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
      if (url === "/api/v2/actions?limit=100") return new Response(JSON.stringify({ data: [] }), { status: 200 });
      if (url === "/api/v2/actions" && init?.method === "POST") {
        return new Response(JSON.stringify({ error: "validation", message: "Target is no longer accessible" }), { status: 403 });
      }
      throw new Error("unexpected fetch");
    });
    vi.stubGlobal("fetch", fetchMock);
    await act(async () => root.render(<SavedActionsCard assets={assets} />));
    await settle();
    await click(button("create", container));
    const dialog = document.body.querySelector<HTMLElement>('[role="dialog"]');
    if (!dialog) throw new Error("missing create dialog");
    const inputs = dialog.querySelectorAll<HTMLInputElement>("input");
    const textareas = dialog.querySelectorAll<HTMLTextAreaElement>("textarea");
    await setInput(inputs[0]!, "Blocked action");
    await setTextarea(textareas[1]!, "uptime");
    const form = dialog.querySelector("form");
    if (!form) throw new Error("missing create form");
    await act(async () => form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })));
    await settle();
    expect(document.body.querySelector('[role="dialog"]')).toBeTruthy();
    expect(dialog.textContent).toContain("Target is no longer accessible");
  });
});

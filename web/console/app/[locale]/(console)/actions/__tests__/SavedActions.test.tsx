import { act, type AnchorHTMLAttributes } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider } from "../../../../contexts/ToastContext";
import type { RowMenuItem } from "../_parts/RowMenu";

// Risks: a saved action is created with the wrong steps, a run reports an exit code the device never sent, a delete
// goes through without the typed name, or a refused create closes the form and loses the hub's reason.

vi.mock("next-intl", () => ({
  useLocale: () => "en",
  useTranslations: () => (key: string, values?: Record<string, unknown>) => {
    if (!values) return key;
    return `${key} ${Object.values(values).join(" ")}`;
  },
}));

vi.mock("../../../../../i18n/navigation", () => ({
  Link: ({ href, ...props }: AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => <a href={href} {...props} />,
}));

const assets = [
  { id: "node-a", name: "Alpha", type: "host", source: "agent", status: "online", last_seen_at: "2026-07-15T00:00:00Z" },
  { id: "node-b", name: "Beta", type: "host", source: "agent", status: "online", last_seen_at: "2026-07-15T00:00:00Z" },
];

vi.mock("../../../../hooks/useActions", () => ({
  useActions: () => ({
    assets,
    connectors: [],
    connectorActions: [],
    selectedConnector: "",
    actionRuns: [],
    actionParameters: [],
    actionParamValues: {},
    actionSubmitting: false,
  }),
}));

const session = vi.hoisted(() => ({ role: "owner" }));
vi.mock("../../../../contexts/AuthContext", () => ({ useAuth: () => ({ user: { id: "u1", username: "u1", role: session.role } }) }));

// The ⋯ menu as plain buttons: the page's behaviour is under test here, not the menu library.
vi.mock("../_parts/RowMenu", async (original) => ({
  ...(await original<typeof import("../_parts/RowMenu")>()),
  RowMenu: ({ items }: { items: RowMenuItem[] }) => (
    <span>{items.map((item) => ("separator" in item ? null : <button key={item.label} type="button" onClick={item.onSelect}>{item.label}</button>))}</span>
  ),
}));

import ActionsPage from "../page";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

const envelope = (data: unknown, status = 200) => new Response(JSON.stringify({ data }), { status, headers: { "Content-Type": "application/json" } });
const settle = () => act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });

async function click(element: Element) {
  await act(async () => { element.dispatchEvent(new MouseEvent("click", { bubbles: true })); });
  await settle();
}

async function type(element: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement, value: string) {
  const proto = Object.getPrototypeOf(element) as object;
  await act(async () => {
    Object.getOwnPropertyDescriptor(proto, "value")?.set?.call(element, value);
    element.dispatchEvent(new Event(element instanceof HTMLSelectElement ? "change" : "input", { bubbles: true }));
  });
}

function button(label: string, scope: ParentNode = document.body): HTMLButtonElement {
  const match = [...scope.querySelectorAll<HTMLButtonElement>("button")].find((b) => b.textContent?.trim() === label || b.getAttribute("aria-label") === label);
  if (!match) throw new Error(`missing button ${label}`);
  return match;
}

const field = <T extends Element>(selector: string) => {
  const found = document.querySelector<T>(selector);
  if (!found) throw new Error(`missing ${selector}`);
  return found;
};

const render = async () => {
  await act(async () => root.render(<ToastProvider><ActionsPage /></ToastProvider>));
  await settle();
};

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  session.role = "owner";
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("Saved actions", () => {
  it("creates, runs with real exit codes, reloads its steps and deletes only after the typed name", async () => {
    let stored: Record<string, unknown> | null = null;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = (init?.method ?? "GET").toUpperCase();
      if (url === "/api/v2/actions?limit=100") return envelope(stored ? [stored] : []);
      if (url === "/api/v2/actions" && method === "POST") {
        stored = { ...JSON.parse(String(init?.body)), id: "act-1", created_by: "owner-1", created_at: "2026-07-15T00:00:00Z" };
        return envelope(stored, 201);
      }
      if (url === "/api/v2/actions/act-1" && method === "GET") return envelope(stored);
      if (url === "/api/v2/actions/act-1/run") {
        return envelope({ action_id: "act-1", steps: [
          { name: "Check Alpha", target: "node-a", status: "succeeded", exit_code: 0, output: "alpha ok", duration_ms: 12 },
          { name: "Check Beta", target: "node-b", error: "exec_failed", message: "beta failed" },
          { name: "Check Gamma", target: "node-a", status: "failed", exit_code: 2, output: "gamma missing" },
          { name: "Check Delta", target: "node-b", status: "failed", output: "command timed out" },
        ] });
      }
      if (url === "/api/v2/actions/act-1" && method === "DELETE") {
        stored = null;
        return envelope({ status: "deleted" });
      }
      throw new Error(`unexpected fetch ${method} ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    await render();
    expect(container.textContent).toContain("empty");

    await click(button("new"));
    await type(field<HTMLInputElement>("#af-name"), "Fleet health check");
    await type(field<HTMLInputElement>("#af-desc"), "Checks both lab devices");
    await type(field<HTMLSelectElement>("#af-s0-dev"), "node-a");
    await type(field<HTMLTextAreaElement>("#af-s0-cmd"), "uptime");
    await click(button("addStep"));
    await type(field<HTMLSelectElement>("#af-s1-dev"), "node-b");
    await type(field<HTMLTextAreaElement>("#af-s1-cmd"), "hostname");
    await act(async () => { field("form").dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })); });
    await settle();

    const create = fetchMock.mock.calls.find((call) => call[0] === "/api/v2/actions" && call[1]?.method === "POST");
    expect(JSON.parse(String(create?.[1]?.body))).toEqual({
      name: "Fleet health check",
      description: "Checks both lab devices",
      steps: [
        { name: "defaultStepName 1", command: "uptime", target: "node-a" },
        { name: "defaultStepName 2", command: "hostname", target: "node-b" },
      ],
    });
    expect(document.querySelector("form")).toBeNull();
    const list = field<HTMLElement>('[aria-label="label"]');
    expect(list.textContent).toContain("Fleet health check");

    // Two devices: Run asks first.
    await click(button("runLabel Fleet health check"));
    const ask = field<HTMLElement>('[role="dialog"]');
    expect(ask.textContent).toContain("Alpha, Beta");
    await click(button("run.confirm 2", ask));
    expect(fetchMock).toHaveBeenCalledWith("/api/v2/actions/act-1/run", { method: "POST", cache: "no-store" });
    expect(list.textContent).toContain("alpha ok");
    expect(list.textContent).toContain("beta failed");
    expect(list.textContent).toContain("exit 0");
    expect(list.textContent).toContain("exit 2");
    expect(list.textContent).not.toContain("exit 1");
    expect(document.body.textContent).toContain("run.finishedFailures Fleet health check 3");

    // Opening the action again loads its current steps.
    await click(button("Fleet health check"));
    await click(button("Fleet health check"));
    expect(fetchMock).toHaveBeenCalledWith("/api/v2/actions/act-1", { cache: "no-store" });

    await click(button("deleteMenu", list));
    const dialog = field<HTMLElement>('[role="dialog"]');
    const confirm = button("delete", dialog);
    expect(confirm.disabled).toBe(true);
    await type(field<HTMLInputElement>("#au-del-name"), "Fleet health check");
    expect(confirm.disabled).toBe(false);
    await click(confirm);
    expect(fetchMock).toHaveBeenCalledWith("/api/v2/actions/act-1", { method: "DELETE", cache: "no-store" });
    expect(list.textContent).not.toContain("Fleet health check");
  });

  it("keeps the form open with the hub's reason when it refuses the action", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input) === "/api/v2/actions?limit=100") return envelope([]);
      if (String(input) === "/api/v2/actions" && init?.method === "POST") {
        return new Response(JSON.stringify({ error: "validation", message: "Target is no longer accessible" }), { status: 403 });
      }
      throw new Error("unexpected fetch");
    }));
    await render();
    await click(button("new"));
    await type(field<HTMLInputElement>("#af-name"), "Blocked action");
    await type(field<HTMLTextAreaElement>("#af-s0-cmd"), "uptime");
    await act(async () => { field("form").dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })); });
    await settle();
    expect(field<HTMLElement>("form").textContent).toContain("Target is no longer accessible");
  });

  // Risk: a viewer is offered New action, Run and Delete, and each one ends in the Hub's refusal.
  it("offers a viewer the steps of a saved action but no changes", async () => {
    session.role = "viewer";
    const saved = { id: "act-1", name: "Fleet health check", steps: [{ name: "Check Alpha", target: "node-a", command: "uptime" }], created_by: "owner-1", created_at: "2026-07-15T00:00:00Z" };
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      if (String(input) === "/api/v2/actions?limit=100") return envelope([saved]);
      throw new Error(`unexpected fetch ${String(input)}`);
    }));
    await render();
    expect(container.textContent).toContain("Fleet health check");
    const labels = [...container.querySelectorAll("button")].map((b) => b.textContent?.trim() || b.getAttribute("aria-label"));
    expect(labels).not.toContain("new");
    expect(labels.filter((label) => label?.startsWith("run") || label === "duplicate" || label === "deleteMenu")).toEqual([]);
    expect(labels).toContain("showSteps");
  });
});

import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NextIntlClientProvider } from "next-intl";

import common from "../../../../../../messages/en/common.json";
import messages from "../../../../../../messages/en/devices.json";

// Controls inside a tab that is offered for listing only, the Hub SSH key card, and the Hub's 422 in a list tab.

const mocks = vi.hoisted(() => ({
  sshSetting: { enabled: false } as { enabled: boolean } | null,
}));

vi.mock("../systemViewParts", async (importOriginal) => {
  const original = await importOriginal<typeof import("../systemViewParts")>();
  // The row ⋯ opens in a portal; list its items as plain text so the test can read them.
  return {
    ...original,
    RowMenu: ({ items }: { items: Array<"sep" | { label: string }> }) => (
      <ul data-testid="row-menu">{items.map((item, i) => (item === "sep" ? null : <li key={i}>{item.label}</li>))}</ul>
    ),
  };
});

vi.mock("../../../../../contexts/AuthContext", () => ({ useAuth: () => ({ user: { role: "owner" } }) }));
vi.mock("../useHubSSHKeySetting", () => ({
  useHubSSHKeySetting: () => ({
    setting: mocks.sshSetting, loading: false, saving: false, error: null, lastChange: null, setEnabled: vi.fn(),
  }),
}));

import { HubSSHKeyCard } from "../HubSSHKeyCard";
import { PackagesTab } from "../PackagesTab";
import { ServicesTab } from "../ServicesTab";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

function answer(status: number, body: unknown) {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })));
}

async function render(node: ReactNode) {
  await act(async () => {
    root.render(<NextIntlClientProvider locale="en" messages={{ common, devices: messages }}>{node}</NextIntlClientProvider>);
  });
  await act(async () => { await new Promise<void>((resolve) => window.setTimeout(resolve, 0)); });
}

const menuText = () => [...container.querySelectorAll('[data-testid="row-menu"] li')].map((li) => li.textContent);

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  mocks.sshSetting = { enabled: false };
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe("ServicesTab", () => {
  const services = { services: [{ name: "nginx.service", description: "web", active_state: "active", sub_state: "running", enabled: "enabled", load_state: "loaded" }] };

  // Fault: Start, Stop or Restart is offered to an agent that lists services but refuses actions (422).
  it("offers only Copy name when the agent does not run service actions", async () => {
    answer(200, services);
    await render(<ServicesTab nodeId="rust-node-01" canAct={false} />);
    expect(menuText()).toEqual(["Copy name"]);
  });

  // Fault: a Go agent device loses its service actions.
  it("keeps Restart and Stop by default", async () => {
    answer(200, services);
    await render(<ServicesTab nodeId="web-01" />);
    expect(menuText()).toEqual(["Restart", "Stop", "Copy name"]);
  });

  // Fault: the Hub's 422 shows as raw text or a generic failure instead of one plain line.
  it("shows the Hub's 422 as a plain line under the load error", async () => {
    answer(422, { error: "this device's agent does not support this" });
    await render(<ServicesTab nodeId="mac-mon-01" />);
    const alert = container.querySelector('[role="alert"]');
    expect(alert?.textContent).toContain(messages.system.units.loadError);
    expect(alert?.textContent).toContain(messages.agentOffers.notOffered);
    expect(alert?.textContent).not.toContain("does not support this");
  });
});

describe("PackagesTab", () => {
  const packages = { packages: [{ name: "curl", version: "8.9.1", status: "installed" }] };
  const upgradeAll = () => [...container.querySelectorAll("button")].some((b) => b.textContent?.includes(messages.system.packages.upgradeAll));

  // Fault: install, remove and upgrade controls show for an agent that lists packages but refuses actions.
  it("hides Upgrade all, the install form and row actions when the agent does not run package actions", async () => {
    answer(200, packages);
    await render(<PackagesTab nodeId="rust-node-01" canAct={false} />);
    expect(container.textContent).toContain("curl");
    expect(upgradeAll()).toBe(false);
    expect(container.querySelector("form")).toBeNull();
    expect(menuText()).toEqual(["Copy name"]);
  });

  // Fault: a Go agent device loses its package controls.
  it("keeps every control by default", async () => {
    answer(200, packages);
    await render(<PackagesTab nodeId="web-01" />);
    expect(upgradeAll()).toBe(true);
    expect(container.querySelector("form")).not.toBeNull();
    expect(menuText()).toEqual(["Upgrade", "Copy name", "Remove…"]);
  });
});

describe("HubSSHKeyCard", () => {
  const title = () => container.textContent?.includes(messages.hubSshKey.title) ?? false;

  // Fault: the card offers to turn on a key the agent cannot install (the Hub refuses with 422).
  it("is left out when the agent does not install the key", async () => {
    await render(<HubSSHKeyCard nodeId="mac-mon-01" isAgentDevice offered={false} />);
    expect(title()).toBe(false);
  });

  // Fault: a key left on from an earlier agent can no longer be turned off.
  it("stays while the key is on, so it can be turned off", async () => {
    mocks.sshSetting = { enabled: true };
    await render(<HubSSHKeyCard nodeId="mac-mon-01" isAgentDevice offered={false} />);
    expect(title()).toBe(true);
  });

  // Fault: the card disappears for devices that keep today's rules.
  it("shows by default", async () => {
    await render(<HubSSHKeyCard nodeId="web-01" isAgentDevice />);
    expect(title()).toBe(true);
  });
});

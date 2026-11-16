import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NextIntlClientProvider } from "next-intl";

import messages from "../../../../../messages/en/devices.json";
import type { AgentAdvertisements } from "../../../../console/agentOffers";
import { agentAsset, goAgentMetadata, monitoringAd, monitoringMetadata } from "../../../../console/__tests__/agentOffersFixtures";
import { createDeviceActionsProvider } from "../../../../components/palette/providers/device-actions";
import type { FastStatusSlice } from "../../../../contexts/StatusContext";

// "Monitoring only" in the Devices list and the device header, the list row's ⋯ and the command palette's device
// actions, for the monitoring edition next to a Go agent device.

const mocks = vi.hoisted(() => ({ ads: {} as AgentAdvertisements, connected: new Set<string>() }));

vi.mock("../../../../contexts/StatusContext", () => ({ useFastStatus: () => ({ agentAdvertisements: mocks.ads }) }));
vi.mock("../../../../hooks/useConnectedAgents", () => ({ useConnectedAgents: () => ({ connectedAgentIds: mocks.connected }) }));
vi.mock("../../../../../i18n/navigation", () => ({
  Link: ({ children, href, className }: { children: ReactNode; href: string; className?: string }) => <a href={href} className={className}>{children}</a>,
  useRouter: () => ({ push: vi.fn() }),
}));
vi.mock("../[id]/DeviceMenu", () => ({
  DeviceMenu: ({ offers }: { offers: Record<string, boolean> }) => (
    <span data-testid="row-offers">{Object.entries(offers).filter(([, on]) => on).map(([k]) => k).join(",")}</span>
  ),
}));

import { DeviceIdentityBar } from "../../../../components/DeviceIdentityBar";
import { DeviceRow } from "../DeviceRow";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const mon = agentAsset("mac-mon-01", "darwin", { ...monitoringMetadata, os_name: "macOS 15.4" });
const goMac = agentAsset("mac-studio-01", "darwin", { ...goAgentMetadata.darwin, os_name: "macOS 15.4" });
const card = (asset: typeof mon) => ({
  asset, freshness: "online" as const, cpu: 12, mem: 40, disk: 55,
  workloads: { containers: 0 } as never, hostedOn: null, dockerHost: null, haHub: null,
});

let container: HTMLDivElement;
let root: Root;

async function render(node: ReactNode) {
  await act(async () => {
    root.render(<NextIntlClientProvider locale="en" timeZone="UTC" messages={{ devices: messages }}>{node}</NextIntlClientProvider>);
  });
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  mocks.ads = { "mac-mon-01": monitoringAd };
  mocks.connected = new Set(["mac-mon-01", "mac-studio-01"]);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

describe("DeviceRow", () => {
  // Fault: the monitoring edition is not labelled in the list, or its ⋯ offers Terminal, Files or Remote view.
  it("labels the monitoring edition and offers it nothing to connect to", async () => {
    await render(<DeviceRow card={card(mon)} />);
    expect(container.querySelector(".ld-sub")?.textContent).toBe(`macOS 15.4 · ${messages.agentOffers.monitoringOnly}`);
    expect(container.querySelector('[data-testid="row-offers"]')?.textContent).toBe("");
  });

  // Fault: a Go agent device gains the label or loses a connect item.
  it("leaves a Go agent device as it was", async () => {
    await render(<DeviceRow card={card(goMac)} />);
    expect(container.querySelector(".ld-sub")?.textContent).toBe("macOS 15.4");
    expect(container.querySelector('[data-testid="row-offers"]')?.textContent).toBe("terminal,files,desktop");
  });
});

describe("DeviceIdentityBar", () => {
  // Fault: the header loses the address or OS, or the label is not shown.
  it("puts the note after the OS", async () => {
    await render(<DeviceIdentityBar asset={{ ...mon, metadata: { ...mon.metadata, ip_address: "192.0.2.10" } }} note={messages.agentOffers.monitoringOnly} />);
    expect(container.querySelector(".dv-sub")?.textContent).toBe(`192.0.2.10 · macOS 15.4 · ${messages.agentOffers.monitoringOnly}`);
  });
});

describe("command palette device actions", () => {
  // Fault: the palette opens Terminal, Desktop or Files for an agent that offers none of them.
  it("leaves out Terminal, Desktop and Files the agent does not offer, and keeps them for Go agents", () => {
    const status = { assets: [mon, goMac], agentAdvertisements: { "mac-mon-01": monitoringAd } } as unknown as FastStatusSlice;
    const items = createDeviceActionsProvider(() => status, vi.fn(), true).search("");
    const keys = (id: string) => items.filter((item) => item.id.startsWith(`device-action-${id}-`)).map((item) => item.id.split("-").pop());
    expect(keys("mac-mon-01")).toEqual(["details", "logs"]);
    expect(keys("mac-studio-01")).toEqual(["terminal", "desktop", "details", "files", "logs"]);
  });
});

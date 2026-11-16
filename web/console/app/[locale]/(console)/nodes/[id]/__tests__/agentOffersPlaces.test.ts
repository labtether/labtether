import { describe, expect, it } from "vitest";

import { deviceAgentOffers, type AgentAdvertisement } from "../../../../../console/agentOffers";
import type { Asset } from "../../../../../console/models";
import { agentAsset, goAgentMetadata, monitoringAd, monitoringMetadata, partialAd, partialMetadata } from "../../../../../console/__tests__/agentOffersFixtures";
import { buildAvailablePanels } from "../devicePanels";
import { rowOffers } from "../deviceMenuModel";
import { withheldAgentPanel } from "../deviceTabsModel";
import { buildNodePanelContext, deriveShouldShowDesktop } from "../nodeDetailPageModelBuilders";
import { computeAvailableDesktopProtocols } from "../useDesktopProtocolState";

// Each place on the device page and the Devices list that decides whether to show an agent tab, button or menu item
// reads console/agentOffers. These pin the three device kinds: the monitoring edition, an explicit agent that offers
// more, and a Go agent (unchanged), plus a device with no agent.

type Kind = { asset: Asset; hasAgent: boolean; ad: AgentAdvertisement | null };
const kinds: Record<string, Kind> = {
  monitoring: { asset: agentAsset("mac-mon-01", "darwin", monitoringMetadata), hasAgent: true, ad: monitoringAd },
  partial: { asset: agentAsset("rust-node-01", "linux", partialMetadata), hasAgent: true, ad: partialAd },
  goLinux: { asset: agentAsset("web-01", "linux", goAgentMetadata.linux), hasAgent: true, ad: null },
  goWindows: { asset: agentAsset("win-desk-01", "windows", goAgentMetadata.windows), hasAgent: true, ad: null },
  goMac: { asset: agentAsset("mac-studio-01", "darwin", goAgentMetadata.darwin), hasAgent: true, ad: null },
  noAgent: { asset: { ...agentAsset("nas01", "linux", {}), source: "manual" }, hasAgent: false, ad: null },
};

function offersOf({ asset, hasAgent, ad }: Kind) {
  return deviceAgentOffers({ metadata: asset.metadata, hasAgent, platform: asset.platform, advertisement: ad });
}

function panelIds(kind: Kind): string[] {
  const offers = offersOf(kind);
  const ctx = buildNodePanelContext({
    asset: kind.asset,
    nodeHasAgent: kind.hasAgent,
    agentOffers: offers,
    mergedDockerHost: null,
    infraCategories: new Map(),
    isProxmoxAsset: false,
    isTrueNASAsset: false,
    isPBSAsset: false,
    isInfra: false,
    isLinuxAgentNode: kind.hasAgent && kind.asset.platform === "linux",
    supportsServiceListing: offers.services,
    supportsPackageListing: offers.packages,
    supportsScheduleListing: offers.schedules,
    supportsNetworkListing: offers.network,
    shouldShowDesktop: deriveShouldShowDesktop({
      assetType: kind.asset.type, isDockerContainer: false, isInfra: false, isDockerHost: false, isProxmoxAsset: false, effectiveKind: "",
    }),
    protocolCount: 0,
    webServiceCount: 0,
    connectionBadges: [],
  });
  return buildAvailablePanels(ctx).map((panel) => panel.id);
}

const GO_PANELS = ["system", "terminal", "desktop", "monitoring", "processes", "network", "files", "services", "packages", "logs", "actions", "cron", "disks", "users", "agent-settings", "connect"];

describe("buildCorePanels (tabs and panels)", () => {
  // Fault: the monitoring edition shows Terminal, Files, Processes, Users, Agent Settings or Remote View.
  it("gives the monitoring edition only System, Monitoring, Network, Logs, Actions, Disks and Connect", () => {
    expect(panelIds(kinds.monitoring)).toEqual(["system", "monitoring", "network", "logs", "actions", "disks", "connect"]);
  });

  // Fault: an explicit agent's offered panels are dropped, or unoffered ones (Services, Users, Remote View) stay.
  it("gives an explicit agent its offered panels only", () => {
    expect(panelIds(kinds.partial)).toEqual(["system", "terminal", "monitoring", "processes", "network", "files", "logs", "actions", "disks", "connect"]);
  });

  // Fault: a Go agent device changes on any platform.
  it.each(["goLinux", "goWindows", "goMac"])("keeps every panel for %s", (name) => {
    expect(panelIds(kinds[name])).toEqual(GO_PANELS);
  });

  // Fault: a device without an agent gains agent panels.
  it("gives a device without an agent no agent panels", () => {
    expect(panelIds(kinds.noAgent)).toEqual(["system", "desktop", "monitoring", "logs", "actions", "connect"]);
  });
});

describe("rowOffers (Devices list row ⋯)", () => {
  // Fault: the list row offers Terminal, Files or Remote view that the agent does not offer, or loses them for Go.
  it("follows the agent's offers", () => {
    const row = (kind: Kind) => rowOffers(kind.asset, kind.hasAgent, kind.ad);
    expect(row(kinds.monitoring)).toEqual({ terminal: false, files: false, desktop: false });
    expect(row(kinds.partial)).toEqual({ terminal: true, files: true, desktop: false });
    expect(row(kinds.goLinux)).toEqual({ terminal: true, files: true, desktop: true });
    expect(row(kinds.goMac)).toEqual({ terminal: true, files: true, desktop: true });
    expect(row(kinds.noAgent)).toEqual({ terminal: false, files: false, desktop: true });
  });
});

describe("withheldAgentPanel (a panel opened by address)", () => {
  // Fault: ?panel=terminal on the monitoring edition loads a terminal that can never start, or a Go device's
  // panel is withheld.
  it("withholds only an explicit agent's unoffered panels", () => {
    const none = new Set<string>();
    const mon = offersOf(kinds.monitoring);
    expect(["terminal", "files", "desktop", "processes", "users", "services", "packages", "cron", "agent-settings"]
      .filter((panel) => withheldAgentPanel(panel, mon, none))).toHaveLength(9);
    expect(withheldAgentPanel("disks", mon, none)).toBe(false);
    expect(withheldAgentPanel("system", mon, none)).toBe(false);
    expect(withheldAgentPanel("terminal", offersOf(kinds.partial), none)).toBe(false);
    expect(withheldAgentPanel("terminal", offersOf(kinds.noAgent), none)).toBe(false);
    expect(withheldAgentPanel("users", offersOf(kinds.goLinux), none)).toBe(false);
    expect(withheldAgentPanel("services", mon, new Set(["services"]))).toBe(false);
  });
});

describe("computeAvailableDesktopProtocols (Remote View choices)", () => {
  const withWebRTC = (kind: Kind): Asset => ({ ...kind.asset, metadata: { ...kind.asset.metadata, webrtc_available: "true" } });
  const protocols = (kind: Kind) => {
    const offers = offersOf(kind);
    return computeAvailableDesktopProtocols(withWebRTC(kind), kind.hasAgent, null, offers.explicit ? offers : null);
  };

  // Fault: an explicit agent is offered VNC or WebRTC through an agent that refuses both.
  it("leaves out VNC and WebRTC that an explicit agent does not offer, and keeps RDP", () => {
    expect(protocols(kinds.partial)).toEqual([]);
    const winExplicit: Kind = { ...kinds.goWindows, ad: { explicit: true, edition: "", offered: ["webrtc.start"] } };
    expect(protocols(winExplicit)).toEqual(["webrtc", "rdp"]);
  });

  // Fault: Go devices lose a protocol.
  it("keeps today's choices for Go agents", () => {
    expect(protocols(kinds.goLinux)).toEqual(["vnc", "webrtc"]);
    expect(protocols(kinds.goWindows)).toEqual(["webrtc", "rdp", "vnc"]);
  });
});

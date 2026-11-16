import { describe, expect, it } from "vitest";

import {
  deviceAgentOffers,
  isAgentUnsupported,
  parseAgentAdvertisement,
  parseAgentAdvertisements,
  type AgentFeature,
  type DeviceAgentOffers,
} from "../agentOffers";
import { goAgentMetadata, monitoringAd, monitoringMetadata, partialAd, partialMetadata } from "./agentOffersFixtures";

const ALL_FEATURES: AgentFeature[] = [
  "terminal", "files", "processes", "disks", "users", "agentSettings", "power", "network", "networkActions",
  "services", "serviceActions", "packages", "packageActions", "schedules", "logQuery", "sshKeys", "vnc", "webrtc",
  "remoteView",
];

const on = (offers: DeviceAgentOffers) => ALL_FEATURES.filter((feature) => offers[feature]).sort();

describe("deviceAgentOffers for an explicit agent", () => {
  // Fault: an explicit device still shows every agent tab, and each one ends in the Hub's 422.
  it("offers the monitoring edition only its disk and network lists, and labels it", () => {
    const offers = deviceAgentOffers({ metadata: monitoringMetadata, hasAgent: true, platform: "darwin", advertisement: monitoringAd });
    expect(on(offers)).toEqual(["disks", "network"]);
    expect(offers.explicit).toBe(true);
    expect(offers.monitoringOnly).toBe(true);
  });

  // Fault: an explicit agent that offers more is cut back to nothing, or keeps power because it is connected.
  it("offers an explicit agent what the Hub lists and nothing else", () => {
    const offers = deviceAgentOffers({ metadata: partialMetadata, hasAgent: true, platform: "linux", advertisement: partialAd });
    expect(on(offers)).toEqual(["disks", "files", "network", "processes", "terminal"]);
    expect(offers.monitoringOnly).toBe(false);
  });

  // Fault: an empty cap_* value counts as supported for an explicit agent too (the Go agent's legacy default).
  it("does not count a missing or empty cap_* value as supported", () => {
    const offers = deviceAgentOffers({
      metadata: { ...partialMetadata, cap_services: "", cap_logs: "" },
      hasAgent: true,
      platform: "linux",
      advertisement: partialAd,
    });
    expect(offers.services).toBe(false);
    expect(offers.logQuery).toBe(false);
  });

  // Fault: an offered feature shows while the agent is not connected, where every request answers 409.
  it("needs the agent connected for what it offers", () => {
    const offers = deviceAgentOffers({ metadata: partialMetadata, hasAgent: false, platform: "linux", advertisement: partialAd });
    expect(offers.terminal).toBe(false);
    expect(offers.disks).toBe(false);
  });

  // Fault: Remote View disappears from an explicit Windows device, though RDP goes through the Hub's gateway.
  it("keeps Remote View for Windows through RDP but not VNC or WebRTC", () => {
    const offers = deviceAgentOffers({ metadata: {}, hasAgent: true, platform: "windows", advertisement: { explicit: true, edition: "", offered: [] } });
    expect(offers.remoteView).toBe(true);
    expect(offers.vnc).toBe(false);
    expect(offers.webrtc).toBe(false);
  });
});

describe("deviceAgentOffers for every other device", () => {
  // Fault: a Go agent device loses or gains anything compared with the console before explicit agents.
  it.each([
    ["linux", ["agentSettings", "disks", "files", "logQuery", "network", "networkActions", "packageActions", "packages", "power", "processes", "remoteView", "schedules", "serviceActions", "services", "sshKeys", "terminal", "users", "vnc", "webrtc"]],
    ["windows", ["agentSettings", "disks", "files", "logQuery", "network", "networkActions", "packageActions", "packages", "power", "processes", "remoteView", "schedules", "serviceActions", "services", "sshKeys", "terminal", "users", "vnc", "webrtc"]],
    ["darwin", ["agentSettings", "disks", "files", "logQuery", "network", "networkActions", "packageActions", "packages", "power", "processes", "remoteView", "schedules", "serviceActions", "services", "sshKeys", "terminal", "users", "vnc", "webrtc"]],
  ] as const)("keeps today's result for a connected Go agent on %s", (platform, expected) => {
    const offers = deviceAgentOffers({ metadata: goAgentMetadata[platform], hasAgent: true, platform, advertisement: null });
    expect(on(offers)).toEqual([...expected]);
    expect(offers.explicit).toBe(false);
    expect(offers.monitoringOnly).toBe(false);
  });

  // Fault: today's rule changes for Go agents: an empty cap_services (Linux without systemd) must still show Services,
  // and network actions still default to Linux only.
  it("keeps counting a missing or empty cap_* value as supported for a Go agent", () => {
    const offers = deviceAgentOffers({ metadata: { cap_services: "" }, hasAgent: true, platform: "linux", advertisement: null });
    expect(offers.services && offers.packages && offers.schedules && offers.logQuery && offers.network).toBe(true);
    expect(offers.networkActions).toBe(true);
    expect(deviceAgentOffers({ metadata: {}, hasAgent: true, platform: "darwin" }).networkActions).toBe(false);
    expect(deviceAgentOffers({ metadata: { cap_services: "action" }, hasAgent: true, platform: "linux" }).services).toBe(false);
  });

  // Fault: a device without a connected agent shows agent tabs.
  it("offers no agent feature without a connected agent", () => {
    const offers = deviceAgentOffers({ metadata: goAgentMetadata.linux, hasAgent: false, platform: "linux", advertisement: null });
    expect(on(offers)).toEqual(["remoteView", "sshKeys", "vnc", "webrtc"]);
  });

  // Fault: an agent that names its edition without the explicit signal loses features (the edition is a label only).
  it("labels a non-explicit monitoring agent without taking anything away", () => {
    const offers = deviceAgentOffers({ metadata: goAgentMetadata.darwin, hasAgent: true, platform: "darwin", advertisement: { explicit: false, edition: "monitoring", offered: [] } });
    expect(offers.monitoringOnly).toBe(true);
    expect(offers.terminal && offers.power && offers.files).toBe(true);
  });
});

describe("parseAgentAdvertisements", () => {
  // Fault: the console works its own rule out of metadata, or drops the map, so no device is ever explicit.
  it("reads the Hub's map and treats a missing offered list as none (the Hub omits an empty list)", () => {
    const parsed = parseAgentAdvertisements({
      "mon-01": { explicit: true, edition: "monitoring", offered: ["disk.list", " Network.List "] },
      "rust-01": { explicit: true },
    });
    expect(parsed["mon-01"]).toEqual({ explicit: true, edition: "monitoring", offered: ["disk.list", "network.list"] });
    expect(parsed["rust-01"]).toEqual({ explicit: true, edition: "", offered: [] });
  });

  // Fault: unknown or malformed data hides features from a device (it must fall back to today's rules).
  it.each([
    ["a string", "explicit"],
    ["explicit as a string", { explicit: "true", offered: [] }],
    ["offered not a list", { explicit: true, offered: "disk.list" }],
    ["offered with a number", { explicit: true, offered: ["disk.list", 4] }],
    ["null", null],
  ])("treats %s as no advertisement", (_label, entry) => {
    expect(parseAgentAdvertisement(entry)).toBeNull();
    const offers = deviceAgentOffers({
      metadata: goAgentMetadata.linux,
      hasAgent: true,
      platform: "linux",
      advertisement: parseAgentAdvertisements({ "dev-01": entry })["dev-01"],
    });
    expect(offers.explicit).toBe(false);
    expect(offers.terminal).toBe(true);
  });

  it("returns an empty map for anything that is not an object, and keeps an unknown edition unlabelled", () => {
    expect(parseAgentAdvertisements(undefined)).toEqual({});
    expect(parseAgentAdvertisements([{ explicit: true }])).toEqual({});
    expect(parseAgentAdvertisement({ explicit: false, edition: "lite" })?.edition).toBe("");
  });
});

describe("isAgentUnsupported", () => {
  // Fault: the Hub's 422 shows as a generic failure, or a different error is mistaken for it.
  it("knows the v1 text, the v2 code, an Error and a parsed body", () => {
    expect(isAgentUnsupported("this device's agent does not support this")).toBe(true);
    expect(isAgentUnsupported(new Error("this device's agent does not support this"))).toBe(true);
    expect(isAgentUnsupported({ error: "agent_unsupported", message: "this device's agent does not support this" })).toBe(true);
    expect(isAgentUnsupported("agent is not connected")).toBe(false);
    expect(isAgentUnsupported({ error: "power action unsupported on this platform" })).toBe(false);
    expect(isAgentUnsupported(null)).toBe(false);
  });
});

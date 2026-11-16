import { describe, expect, it } from "vitest";

import { remoteDevices } from "../../[locale]/(console)/remote-view/newTabModel";
import { agentWithholds } from "../agentOffers";
import { agentAsset, goAgentMetadata, monitoringAd, monitoringMetadata, partialAd, partialMetadata } from "./agentOffersFixtures";

// The Terminal, Files and Remote View pages list devices to open. They judge an explicit agent from its advertisement
// alone (agentWithholds), because they have no device page model to hand.

describe("agentWithholds", () => {
  // Fault: the Terminal or Files picker lists a monitoring edition device, and opening it ends in the Hub's 422.
  it("withholds what an explicit agent does not offer", () => {
    expect(agentWithholds(monitoringAd, "terminal", "darwin")).toBe(true);
    expect(agentWithholds(monitoringAd, "files", "darwin")).toBe(true);
    expect(agentWithholds(monitoringAd, "remoteView", "darwin")).toBe(true);
    expect(agentWithholds(partialAd, "terminal", "linux")).toBe(false);
    expect(agentWithholds(partialAd, "files", "linux")).toBe(false);
  });

  // Fault: a Go agent device (no advertisement) or one that only names its edition drops out of a picker.
  it("withholds nothing from an agent that is not explicit", () => {
    expect(agentWithholds(null, "terminal", "linux")).toBe(false);
    expect(agentWithholds(undefined, "files", "linux")).toBe(false);
    expect(agentWithholds({ explicit: false, edition: "monitoring", offered: [] }, "terminal", "darwin")).toBe(false);
  });

  // Fault: an explicit Windows device loses Remote View, though RDP goes through the Hub's gateway.
  it("keeps Remote View for an explicit Windows agent", () => {
    expect(agentWithholds({ explicit: true, edition: "", offered: [] }, "remoteView", "windows")).toBe(false);
  });
});

describe("remoteDevices", () => {
  const assets = [
    agentAsset("mac-mon-01", "darwin", monitoringMetadata),
    agentAsset("rust-node-01", "linux", partialMetadata),
    agentAsset("web-01", "linux", goAgentMetadata.linux),
  ];

  // Fault: Remote View's new tab lists the monitoring edition device, which offers neither VNC nor WebRTC.
  it("leaves out an explicit agent without Remote View and keeps the rest", () => {
    const ids = remoteDevices(assets, { "mac-mon-01": monitoringAd, "rust-node-01": partialAd }).map((d) => d.id);
    expect(ids).toEqual(["web-01"]);
  });

  // Fault: a Go-only fleet (no map) loses devices from Remote View.
  it("lists every device when no agent lists what it offers", () => {
    expect(remoteDevices(assets).map((d) => d.id)).toEqual(["mac-mon-01", "rust-node-01", "web-01"]);
  });
});

import { describe, expect, it } from "vitest";

import { canSaveQuickConnectBookmark, emptyDraft, toConnectParams, withHostInput, withProtocol } from "../quickConnectModel";

// Risk: session-only RDP security choices (untrusted or pinned certificate, old RDP security) must never be stored in
// a bookmark, and one protocol's security choices must not leak into a connection made with another protocol.
describe("Quick connect bookmark safety", () => {
  it("does not save session-only RDP security choices", () => {
    expect(canSaveQuickConnectBookmark("rdp", true, false, "")).toBe(false);
    expect(canSaveQuickConnectBookmark("rdp", false, true, "")).toBe(false);
    expect(canSaveQuickConnectBookmark("rdp", false, false, "sha256:AA:BB")).toBe(false);
  });

  it("still saves connections that use normal protocol defaults", () => {
    expect(canSaveQuickConnectBookmark("rdp", false, false, "")).toBe(true);
    expect(canSaveQuickConnectBookmark("vnc", false, false, "")).toBe(true);
  });

  it("drops the bookmark request when an RDP choice is for this session only", () => {
    const draft = { ...withProtocol(emptyDraft("desk.example.com"), "rdp"), saveBookmark: true, ignoreCertificate: true };
    const params = toConnectParams(draft, true);
    expect(params.ignoreCertificate).toBe(true);
    expect(params.saveBookmark).toBeUndefined();
  });
});

describe("Quick connect protocol switching", () => {
  it("clears RDP and plain VNC choices when switching to SPICE and moves a default port along", () => {
    const rdp = { ...withProtocol(emptyDraft("10.20.0.31"), "rdp"), certificateFingerprints: "sha256:AA", allowInsecureVNC: true };
    const spice = withProtocol(rdp, "spice");
    expect(spice.port).toBe("5930");
    const params = toConnectParams(spice, false);
    expect(params).toEqual({ protocol: "spice", host: "10.20.0.31", port: 5930, spiceSecurityMode: "tls" });
  });

  it("fills protocol, host and port from a pasted address", () => {
    expect(withHostInput(emptyDraft(), "rdp://desk.example.com:3390")).toMatchObject({ protocol: "rdp", host: "desk.example.com", port: "3390" });
  });
});

import { describe, expect, it } from "vitest";

import { profilesOf } from "../useCredentialProfiles";

// The hub's v2 credential list answers { data: { profiles } }. The console read only { profiles }, so on a real hub
// every saved-login list and picker showed "response was invalid" (found on the 2.0 lab Hub, 2026-11-02).
describe("profilesOf", () => {
  it("reads the list from the hub's v2 data envelope", () => {
    const profile = { id: "cred_1", name: "nas01 root", kind: "ssh_password" };
    expect(profilesOf({ data: { profiles: [profile] }, request_id: "r" } as never)).toEqual([profile]);
  });

  it("still reads an unwrapped list", () => {
    expect(profilesOf({ profiles: [] })).toEqual([]);
  });
});

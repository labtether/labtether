import { describe, expect, it } from "vitest";

import {
  buildCredentialCreatePayload,
  credentialReferenceMessage,
  parseCredentialMetadata,
} from "../credentialProfileModel";

describe("credential profile model", () => {
  it("normalizes metadata fields while preserving exact secret and passphrase bytes", () => {
    const payload = buildCredentialCreatePayload({
      name: " Exact key ",
      kind: "ssh_private_key",
      username: " root ",
      description: " key for lab ",
      secret: " \nprivate-key\t ",
      passphrase: " passphrase ",
      metadataText: '{"base_url":"https://lab.example"}',
    });

    expect(payload).toEqual({
      name: "Exact key",
      kind: "ssh_private_key",
      username: "root",
      description: "key for lab",
      secret: " \nprivate-key\t ",
      passphrase: " passphrase ",
      metadata: { base_url: "https://lab.example" },
    });
  });

  it("rejects non-string metadata and formats redacted dependency counts", () => {
    expect(() => parseCredentialMetadata('{"secret":42}')).toThrow("must be a string");
    expect(credentialReferenceMessage([
      { resource: "asset_protocol_configs", count: 2 },
      { resource: "hub_collectors", count: 1 },
    ])).toBe("Remove its live dependencies first: 2 asset protocol configurations, 1 collectors.");
  });
});

import { describe, expect, it } from "vitest";

import {
  buildCredentialCreatePayload,
  credentialReferenceUses,
  CredentialValidationError,
  parseCredentialMetadata,
} from "../credentialProfileModel";

function problemOf(run: () => unknown): unknown {
  try {
    run();
  } catch (error) {
    return error instanceof CredentialValidationError ? { problem: error.problem, values: error.values } : error;
  }
  return null;
}

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

  it("names metadata problems as message keys instead of parser text, and counts unknown references together", () => {
    expect(problemOf(() => parseCredentialMetadata('{"secret":42}'))).toEqual({ problem: "metadataText", values: { key: "secret" } });
    expect(problemOf(() => parseCredentialMetadata("{base_url:"))).toEqual({ problem: "metadataInvalid", values: {} });
    expect(credentialReferenceUses([
      { resource: "asset_protocol_configs", count: 2 },
      { resource: "hub_collectors", count: 1 },
      { resource: "future_table", count: 3 },
      { resource: "another_future_table", count: 1 },
    ])).toEqual([
      { use: "protocolConfigs", count: 2 },
      { use: "collectors", count: 1 },
      { use: "other", count: 4 },
    ]);
  });
});

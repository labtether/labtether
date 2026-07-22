import { describe, expect, it } from "vitest";
import { webhookRecordsFromPayload } from "../webhookPayload";

const webhook = {
  id: "wh-1",
  name: "Operations",
  url: "https://example.invalid/hook",
  events: ["alert.fired"],
  enabled: true,
};

describe("webhookRecordsFromPayload", () => {
  it("accepts the direct array returned by the v2 API", () => {
    expect(webhookRecordsFromPayload([webhook])).toEqual([webhook]);
  });

  it("retains compatibility with an enveloped response", () => {
    expect(webhookRecordsFromPayload({ webhooks: [webhook] })).toEqual([webhook]);
  });

  it("fails closed for malformed payloads", () => {
    expect(webhookRecordsFromPayload({ webhooks: "not-an-array" })).toEqual([]);
    expect(webhookRecordsFromPayload(null)).toEqual([]);
  });
});

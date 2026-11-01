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
  // Risk: the hub's v2 envelope ({ request_id, data }) was read as an empty list, so saved webhooks looked gone.
  it("reads the hub's v2 envelope", () => {
    expect(webhookRecordsFromPayload({ request_id: "req_1", data: [webhook] })).toEqual([webhook]);
  });

  it("accepts a bare array and the older enveloped response", () => {
    expect(webhookRecordsFromPayload([webhook])).toEqual([webhook]);
    expect(webhookRecordsFromPayload({ webhooks: [webhook] })).toEqual([webhook]);
  });

  it("fails closed for malformed payloads", () => {
    expect(webhookRecordsFromPayload({ webhooks: "not-an-array" })).toEqual([]);
    expect(webhookRecordsFromPayload({ data: { id: "wh-1" } })).toEqual([]);
    expect(webhookRecordsFromPayload(null)).toEqual([]);
  });
});

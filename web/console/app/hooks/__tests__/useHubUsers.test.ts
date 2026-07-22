import { describe, expect, it } from "vitest";

import { readUserPayload } from "../useHubUsers";

describe("readUserPayload", () => {
  it("accepts an empty successful 204 response", async () => {
    await expect(readUserPayload(new Response(null, { status: 204 }))).resolves.toEqual({});
  });

  it("preserves a structured upstream error", async () => {
    const response = new Response(JSON.stringify({ error: "cannot delete your own account" }), {
      status: 400,
      headers: { "content-type": "application/json" },
    });
    await expect(readUserPayload(response)).resolves.toEqual({ error: "cannot delete your own account" });
  });

  it("falls back safely when an upstream error is not JSON", async () => {
    await expect(readUserPayload(new Response("bad gateway", { status: 502 }))).resolves.toEqual({});
  });
});

import { describe, expect, it } from "vitest";

import { markResponseNoStore } from "../noStoreResponse";

describe("markResponseNoStore", () => {
  it("prevents caching of OAuth state and session-bearing responses", () => {
    const response = markResponseNoStore(new Response(null, { status: 302 }));

    expect(response.headers.get("cache-control")).toBe("no-store, max-age=0");
    expect(response.headers.get("pragma")).toBe("no-cache");
    expect(response.headers.get("expires")).toBe("0");
  });
});

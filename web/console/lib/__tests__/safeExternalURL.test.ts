import { describe, expect, it } from "vitest";

import { safeHttpURL } from "../safeExternalURL";

// A compromised agent can report any service URL. Only http(s) links may be
// opened from the console.
describe("safeHttpURL", () => {
  it("keeps http and https links", () => {
    expect(safeHttpURL(" https://nas01.local:5001/ ")).toBe("https://nas01.local:5001/");
    expect(safeHttpURL("http://192.0.2.10:8080")).toBe("http://192.0.2.10:8080/");
  });

  it.each(["javascript:alert(1)", "JAVASCRIPT:alert(1)", "data:text/html,hi", "//evil.example", "", undefined])(
    "drops %s",
    (value) => {
      expect(safeHttpURL(value)).toBeUndefined();
    },
  );
});

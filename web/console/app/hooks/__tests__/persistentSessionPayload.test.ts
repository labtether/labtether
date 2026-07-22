import { describe, expect, it } from "vitest";

import { persistentSessionIDFromPayload } from "../persistentSessionPayload";

describe("persistentSessionIDFromPayload", () => {
  it("reads the canonical hub response envelope", () => {
    expect(persistentSessionIDFromPayload({
      persistent_session: { id: " pts-1 " },
    })).toBe("pts-1");
  });

  it("keeps compatibility with legacy envelopes without trusting malformed values", () => {
    expect(persistentSessionIDFromPayload({ session: { id: "pts-2" } })).toBe("pts-2");
    expect(persistentSessionIDFromPayload({ id: "pts-3" })).toBe("pts-3");
    expect(persistentSessionIDFromPayload({ persistent_session: { id: 42 } })).toBe("");
  });
});

import { describe, expect, it } from "vitest";

import type { AgentTokenSummary, EnrollmentToken } from "../../../../../console/models";
import { deadTokenCount, enrollmentTokenState, formatWhen } from "../settingsModel";

const now = new Date(2026, 9, 28, 22, 40);

function token(patch: Partial<EnrollmentToken>): EnrollmentToken {
  return {
    id: "en-1",
    label: "Garage Pi",
    expires_at: new Date(2026, 9, 29, 22, 40).toISOString(),
    max_uses: 1,
    use_count: 0,
    created_at: new Date(2026, 9, 28, 10, 0).toISOString(),
    scope: "asset",
    ...patch,
  };
}

// The Agents page offers Disable only on tokens that still work, and "Clear N dead tokens" must count exactly
// what the hub's cleanup deletes; a wrong count would promise to delete live tokens.
describe("enrollment token state", () => {
  it("names why a token can no longer enroll", () => {
    expect(enrollmentTokenState(token({}), now)).toBe("active");
    expect(enrollmentTokenState(token({ revoked_at: now.toISOString() }), now)).toBe("revoked");
    expect(enrollmentTokenState(token({ expires_at: new Date(2026, 9, 27).toISOString() }), now)).toBe("expired");
    expect(enrollmentTokenState(token({ use_count: 1 }), now)).toBe("usedUp");
    expect(enrollmentTokenState(token({ max_uses: 0, use_count: 9 }), now)).toBe("active");
  });

  it("counts dead enrollment tokens and never-used revoked agent keys", () => {
    const agent = (patch: Partial<AgentTokenSummary>): AgentTokenSummary => ({
      id: "ak", asset_id: "nas01", status: "active", expires_at: "", created_at: now.toISOString(), ...patch,
    });
    const count = deadTokenCount(
      [token({}), token({ id: "b", use_count: 1 }), token({ id: "c", revoked_at: now.toISOString() })],
      [agent({}), agent({ id: "r1", status: "revoked" }), agent({ id: "r2", status: "revoked", last_used_at: now.toISOString() })],
      now,
    );
    expect(count).toBe(3);
  });
});

describe("formatWhen", () => {
  it("shows the time today, weekday and time this week, and a date further away", () => {
    expect(formatWhen(new Date(2026, 9, 28, 9, 5).toISOString(), now, "en-GB")).toBe("09:05");
    expect(formatWhen(new Date(2026, 9, 26, 14, 20).toISOString(), now, "en-GB")).toBe("Mon 14:20");
    expect(formatWhen(new Date(2026, 6, 2, 8, 0).toISOString(), now, "en-GB")).toBe("2 Jul");
    expect(formatWhen(new Date(2027, 9, 3).toISOString(), now, "en-GB")).toBe("3 Oct 2027");
    expect(formatWhen("not a date", now)).toBe("");
  });
});

import { describe, expect, it } from "vitest";

import { parseBackupAgeDays } from "../DeviceOverviewGrid";

describe("parseBackupAgeDays", () => {
  it("accepts strict non-negative decimal day values", () => {
    expect(parseBackupAgeDays("0")).toBe(0);
    expect(parseBackupAgeDays(" 1.5 ")).toBe(1.5);
    expect(parseBackupAgeDays("30")).toBe(30);
  });

  it("rejects malformed numeric-looking values", () => {
    expect(parseBackupAgeDays("1e2")).toBeNull();
    expect(parseBackupAgeDays("30abc")).toBeNull();
    expect(parseBackupAgeDays("+30")).toBeNull();
    expect(parseBackupAgeDays("-1")).toBeNull();
  });
});

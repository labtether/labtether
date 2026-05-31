import { describe, expect, it } from "vitest";

import { parseIntSetting } from "../formatters";

describe("parseIntSetting", () => {
  it("accepts strict positive decimal values", () => {
    expect(parseIntSetting("15", 5)).toBe(15);
    expect(parseIntSetting(" 120 ", 5)).toBe(120);
  });

  it("rejects malformed numeric-looking values", () => {
    expect(parseIntSetting("15junk", 5)).toBe(5);
    expect(parseIntSetting("1e3", 5)).toBe(5);
    expect(parseIntSetting("+15", 5)).toBe(5);
  });

  it("rejects empty and non-positive values", () => {
    expect(parseIntSetting("", 5)).toBe(5);
    expect(parseIntSetting("0", 5)).toBe(5);
  });
});

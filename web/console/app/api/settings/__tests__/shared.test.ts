import { describe, expect, it } from "vitest";

import { clampInterval, numberValue } from "../shared";

describe("settings API numeric helpers", () => {
  it("accepts plain integer strings and finite numbers", () => {
    expect(numberValue("120", 60)).toBe(120);
    expect(numberValue("0015", 60)).toBe(15);
    expect(numberValue(90, 60)).toBe(90);
  });

  it("rejects non-decimal integer strings before interval clamping", () => {
    expect(clampInterval(numberValue("1e3", 60))).toBe(60);
    expect(clampInterval(numberValue("30abc", 60))).toBe(60);
    expect(clampInterval(numberValue("15.5", 60))).toBe(60);
  });

  it("still clamps accepted interval values to the supported range", () => {
    expect(clampInterval(numberValue("5", 60))).toBe(15);
    expect(clampInterval(numberValue("7200", 60))).toBe(3600);
  });
});

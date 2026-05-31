import { describe, expect, it } from "vitest";

import { formatCompatConfidence } from "../servicesLayoutHelpers";

describe("formatCompatConfidence", () => {
  it("formats finite confidence values", () => {
    expect(formatCompatConfidence("0.42")).toBe("42%");
    expect(formatCompatConfidence(" 1 ")).toBe("100%");
  });

  it("rejects malformed numeric-looking values", () => {
    expect(formatCompatConfidence("0.42junk")).toBe("");
    expect(formatCompatConfidence("NaN")).toBe("");
    expect(formatCompatConfidence("Inf")).toBe("");
  });
});

import { describe, expect, it } from "vitest";

import { parsePortInput } from "../portParsing";

describe("parsePortInput", () => {
  it("accepts strict decimal ports in range", () => {
    expect(parsePortInput("22", 3389)).toBe(22);
    expect(parsePortInput(" 3389 ", 22)).toBe(3389);
    expect(parsePortInput("65535", 22)).toBe(65535);
  });

  it("rejects malformed numeric-looking ports", () => {
    expect(parsePortInput("22abc", 3389)).toBe(3389);
    expect(parsePortInput("1e3", 3389)).toBe(3389);
    expect(parsePortInput("+22", 3389)).toBe(3389);
    expect(parsePortInput("２２", 3389)).toBe(3389);
  });

  it("rejects empty and out-of-range ports", () => {
    expect(parsePortInput("", 5900)).toBe(5900);
    expect(parsePortInput("0", 5900)).toBe(5900);
    expect(parsePortInput("65536", 5900)).toBe(5900);
  });

  it("normalizes an invalid fallback to ssh", () => {
    expect(parsePortInput("bad", 0)).toBe(22);
  });
});

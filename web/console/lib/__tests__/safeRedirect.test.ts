import { describe, expect, it } from "vitest";

import { safeLocalRedirectPath } from "../safeRedirect";

describe("safeLocalRedirectPath", () => {
  it.each([
    ["/", "/"],
    ["/en/dashboard?tab=alerts", "/en/dashboard?tab=alerts"],
    ["https://evil.example/path", "/"],
    ["//evil.example/path", "/"],
    ["/\\evil.example/path", "/"],
    ["\\\\evil.example/path", "/"],
    ["/%5cevil.example/path", "/"],
    ["/%2f%2fevil.example/path", "/"],
    ["/safe\nLocation:%20https://evil.example", "/"],
  ])("normalizes %j to %j", (input, expected) => {
    expect(safeLocalRedirectPath(input)).toBe(expected);
  });
});

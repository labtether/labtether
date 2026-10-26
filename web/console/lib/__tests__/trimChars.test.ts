import { describe, expect, it } from "vitest";

import { trimChars, trimTrailingChars, trimTrailingSlashes } from "../trimChars";

// These replace /\/+$/-style regexes that CodeQL flagged as polynomial ReDoS.
// A long run of the trimmed character followed by something else is the input
// that made the regex quadratic; with 200k characters the old pattern would
// run for many seconds, so finishing here at all shows the scan is linear.
describe("trimChars helpers", () => {
  it("trims trailing slashes only at the end", () => {
    expect(trimTrailingSlashes("https://hub.example.com/api///")).toBe("https://hub.example.com/api");
    expect(trimTrailingSlashes("///")).toBe("");
    expect(trimTrailingSlashes("/")).toBe("");
    expect(trimTrailingSlashes("a/b")).toBe("a/b");
    expect(trimTrailingSlashes("")).toBe("");
  });

  it("keeps a long slash run that is followed by another character", () => {
    const input = `${"/".repeat(200_000)}x`;
    expect(trimTrailingSlashes(input)).toBe(input);
    expect(trimTrailingSlashes(`${input}${"/".repeat(200_000)}`)).toBe(input);
  });

  it("trims any of several characters and both edges", () => {
    expect(trimTrailingChars("C:\\Backup\\//", "\\/")).toBe("C:\\Backup");
    expect(trimTrailingChars("host.example.com...", ".")).toBe("host.example.com");
    expect(trimChars("__metric__name__", "_")).toBe("metric__name");
    expect(trimChars("/agent/binary/", "/")).toBe("agent/binary");
  });
});

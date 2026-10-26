import { describe, expect, it } from "vitest";
import { icuArguments } from "../check-i18n-keys.mjs";

// The i18n gate compares these signatures between English and each locale,
// so a parser slip would let a translation silently drop or rename a value.
describe("icuArguments", () => {
  it("treats plural branch text as text, not as arguments", () => {
    expect(icuArguments("{count, plural, one {# asset} other {# assets}}")).toEqual(["count:plural"]);
  });

  it("finds arguments nested inside branches and after them", () => {
    expect(icuArguments("{count, plural, one {# {kind}} other {# {kind}s}} for {name}")).toEqual([
      "count:plural",
      "kind",
      "name",
    ]);
  });

  it("reports a renamed placeholder as a different signature", () => {
    expect(icuArguments("Hallo {nom}")).not.toEqual(icuArguments("Hello {name}"));
  });
});

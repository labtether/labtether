import { describe, expect, it } from "vitest";

import {
  buildLocalSetupDestination,
  buildRemoteAccessURL,
} from "../setupNavigation";

describe("setup completion navigation", () => {
  it("keeps an already localized console path exact", () => {
    expect(buildLocalSetupDestination("/en")).toBe("/en");
    expect(buildLocalSetupDestination("/fr/nodes?view=grid")).toBe(
      "/fr/nodes?view=grid",
    );
  });

  it("fails closed for a non-local return path", () => {
    expect(buildLocalSetupDestination("https://evil.example/")).toBe("/");
    expect(buildLocalSetupDestination("//evil.example/")).toBe("/");
  });

  it("preserves the exact localized path when switching to Tailscale HTTPS", () => {
    expect(
      buildRemoteAccessURL("https://hub.example.ts.net", "/en/settings"),
    ).toBe("https://hub.example.ts.net/en/settings");
  });
});

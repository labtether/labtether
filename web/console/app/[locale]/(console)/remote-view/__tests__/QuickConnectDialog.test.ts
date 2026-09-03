import { describe, expect, it } from "vitest";

import { canSaveQuickConnectBookmark } from "../QuickConnectDialog";

describe("QuickConnectDialog bookmark safety", () => {
  it("does not save session-only RDP security choices", () => {
    expect(canSaveQuickConnectBookmark("rdp", true, false, "")).toBe(false);
    expect(canSaveQuickConnectBookmark("rdp", false, true, "")).toBe(false);
    expect(
      canSaveQuickConnectBookmark("rdp", false, false, "sha256:AA:BB"),
    ).toBe(false);
  });

  it("still saves connections that use normal protocol defaults", () => {
    expect(canSaveQuickConnectBookmark("rdp", false, false, "")).toBe(true);
    expect(canSaveQuickConnectBookmark("vnc", false, false, "")).toBe(true);
  });
});

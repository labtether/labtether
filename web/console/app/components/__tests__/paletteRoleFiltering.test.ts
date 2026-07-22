import { describe, expect, it, vi } from "vitest";

import type { FastStatusSlice } from "../../contexts/StatusContext";
import { createDeviceActionsProvider } from "../palette/providers/device-actions";
import { createNavigationProvider } from "../palette/providers/navigation";
import { createQuickConnectProvider } from "../palette/providers/quick-connect";
import { createSettingsProvider } from "../palette/providers/settings";
import { createSnippetsProvider } from "../palette/providers/snippets";

const status = {
  timestamp: "2026-07-15T00:00:00Z",
  summary: { servicesUp: 1, servicesTotal: 1, assetCount: 1, staleAssetCount: 0 },
  endpoints: [],
  telemetryOverview: [],
  assets: [{ id: "asset-1", name: "QA host", type: "host", status: "online" }],
} as unknown as FastStatusSlice;

describe("palette role filtering", () => {
  it("does not advertise write-only or administrative pages to a viewer", () => {
    const items = createNavigationProvider(vi.fn(), "viewer").search("");
    const hrefs = items.map((item) => item.href);

    expect(hrefs).toContain("/users");
    expect(hrefs).not.toContain("/terminal");
    expect(hrefs).not.toContain("/remote-view");
    expect(hrefs).not.toContain("/settings");
    expect(hrefs).not.toContain("/security");
  });

  it("keeps read-only device actions while hiding session creation from viewers", () => {
    const viewerLabels = createDeviceActionsProvider(() => status, vi.fn(), false)
      .search("")
      .map((item) => item.label);
    const operatorLabels = createDeviceActionsProvider(() => status, vi.fn(), true)
      .search("")
      .map((item) => item.label);

    expect(viewerLabels).toEqual([
      "QA host: Details",
      "QA host: Files",
      "QA host: Logs",
    ]);
    expect(operatorLabels).toContain("QA host: Terminal");
    expect(operatorLabels).toContain("QA host: Desktop");
  });

  it("disables every terminal shortcut provider for a viewer", () => {
    expect(createQuickConnectProvider(vi.fn(), false).search("root@example.test")).toEqual([]);
    expect(createSnippetsProvider(() => [], vi.fn(), false).search("!")).toEqual([]);
  });

  it("keeps account security reachable without exposing hub settings", () => {
    const viewerItems = createSettingsProvider(vi.fn(), "viewer").search("");
    expect(viewerItems.map((item) => item.href)).toEqual(["/users"]);

    const adminItems = createSettingsProvider(vi.fn(), "admin").search("");
    expect(adminItems.map((item) => item.href)).toContain("/settings?tab=advanced");
    expect(adminItems.map((item) => item.href)).toContain("/settings?tab=general");
  });
});

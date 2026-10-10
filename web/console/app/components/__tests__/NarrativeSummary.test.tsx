import type { ComponentProps } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { NarrativeSummary } from "../NarrativeSummary";

type NarrativeStatus = NonNullable<ComponentProps<typeof NarrativeSummary>["status"]>;

function status(): NarrativeStatus {
  const now = new Date().toISOString();
  return {
    timestamp: now,
    summary: {
      servicesUp: 1, servicesTotal: 1, connectorCount: 0, groupCount: 0,
      assetCount: 1, sessionCount: 0, auditCount: 0, processedJobs: 0,
      actionRunCount: 0, updateRunCount: 0, deadLetterCount: 0, staleAssetCount: 0,
    },
    assets: [{ id: "asset-1", name: "functional-linux", type: "host", source: "agent", status: "online", last_seen_at: now }],
    telemetryOverview: [{
      asset_id: "asset-1", name: "functional-linux", type: "host", source: "agent",
      status: "online", last_seen_at: now, metrics: { disk_used_percent: 40 },
    }],
    endpoints: [],
    groupReliability: [],
  } as unknown as NarrativeStatus;
}

function summaryText(value: NarrativeStatus): string {
  const element = document.createElement("div");
  element.innerHTML = renderToStaticMarkup(<NarrativeSummary status={value} />);
  return element.textContent ?? "";
}

describe("dashboard narrative health claim", () => {
  it("shows healthy only when its reported signals have no warnings", () => {
    expect(summaryText(status())).toContain("Your lab is healthy. 1 device online.");
  });

  it("does not call the lab healthy when the same summary reports a critically full disk", () => {
    const snapshot = status();
    snapshot.telemetryOverview[0].metrics.disk_used_percent = 100;

    const text = summaryText(snapshot);
    expect(text).not.toContain("Your lab is healthy");
    expect(text).toContain("functional-linux disk at 100% — critically full.");
  });

  it("does not call the lab healthy when a service endpoint is down", () => {
    const snapshot = status();
    snapshot.endpoints = [{ name: "Gateway", ok: false }] as NarrativeStatus["endpoints"];

    const text = summaryText(snapshot);
    expect(text).not.toContain("Your lab is healthy");
    expect(text).toContain("Gateway endpoint is down.");
  });

  it("does not call the lab healthy when a service is down", () => {
    const snapshot = status();
    snapshot.summary.servicesUp = 1;
    snapshot.summary.servicesTotal = 2;

    const text = summaryText(snapshot);
    expect(text).not.toContain("Your lab is healthy");
    expect(text).toContain("1 of 2 services online.");
  });

  it("does not call the lab healthy when retention reports an error", () => {
    const snapshot = status();
    snapshot.summary.retentionError = "cleanup failed";

    const text = summaryText(snapshot);
    expect(text).not.toContain("Your lab is healthy");
    expect(text).toContain("Retention cleanup needs attention.");
  });
});

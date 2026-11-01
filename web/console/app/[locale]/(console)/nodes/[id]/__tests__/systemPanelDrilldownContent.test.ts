import { describe, expect, it } from "vitest";

import type { Asset } from "../../../../../console/models";
import { buildDrilldownContent } from "../systemPanelDrilldownContent";

// The Storage drill-down must tell "0 bytes free" (a full disk) apart from "free space unknown".
function asset(available?: string): Asset {
  return {
    id: "node-1", type: "host", name: "Node 1", source: "agent",
    status: "online", last_seen_at: "2026-10-09T00:00:00Z",
    metadata: {
      disk_root_total_bytes: "10737418240",
      ...(available === undefined ? {} : { disk_root_available_bytes: available }),
    },
  };
}

describe("storage free space", () => {
  it("shows valid zero available bytes as zero free", () => {
    const drilldown = buildDrilldownContent(asset("0"), { disk_used_percent: 100 }, "storage");
    expect(drilldown.heroHint).toContain("0 MiB free of 10.0 GiB");
  });

  it.each([undefined, "", "bad", "0x0"])("keeps missing or malformed available bytes unknown (%s)", (raw) => {
    const drilldown = buildDrilldownContent(asset(raw), { disk_used_percent: 100 }, "storage");
    expect(drilldown.heroHint).toContain("n/a free of 10.0 GiB");
  });
});

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { Asset } from "../../console/models";
import { buildDrilldownContent } from "../../[locale]/(console)/nodes/[id]/systemPanelDrilldownContent";
import { StorageCard } from "../DeviceOverviewCards";

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
    const value = asset("0");
    expect(renderToStaticMarkup(<StorageCard asset={value} diskPercent={100} />))
      .toContain("0 MB free of 10.0 GB");

    const drilldown = buildDrilldownContent(value, { disk_used_percent: 100 }, "storage");
    expect(drilldown.heroHint).toContain("0 MiB free of 10.0 GiB");
  });

  it.each([undefined, "", "bad", "0x0"])("keeps missing or malformed available bytes unknown (%s)", (raw) => {
    const value = asset(raw);
    const card = renderToStaticMarkup(<StorageCard asset={value} diskPercent={100} />);
    expect(card).toContain("10.0 GB");
    expect(card).not.toContain("free of");

    const drilldown = buildDrilldownContent(value, { disk_used_percent: 100 }, "storage");
    expect(drilldown.heroHint).toContain("n/a free of 10.0 GiB");
  });
});

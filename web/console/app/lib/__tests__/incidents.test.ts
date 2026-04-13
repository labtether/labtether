import { describe, expect, it } from "vitest";

import type { Incident } from "../../console/models";
import { extractIncident, upsertIncident } from "../incidents";

const baseIncident: Incident = {
  id: "inc-1",
  title: "API outage",
  severity: "high",
  status: "investigating",
  source: "manual",
  created_at: "2026-04-12T09:00:00.000Z",
  updated_at: "2026-04-12T09:05:00.000Z",
};

describe("extractIncident", () => {
  it("reads wrapped incident payloads", () => {
    expect(extractIncident({ incident: baseIncident })).toEqual(baseIncident);
  });

  it("reads direct incident payloads", () => {
    expect(extractIncident(baseIncident)).toEqual(baseIncident);
  });

  it("returns null for unrelated payloads", () => {
    expect(extractIncident({ incidents: [baseIncident] })).toBeNull();
  });
});

describe("upsertIncident", () => {
  it("replaces an existing incident and keeps the list ordered by updated_at", () => {
    const olderIncident: Incident = {
      ...baseIncident,
      id: "inc-2",
      title: "Disk pressure",
      updated_at: "2026-04-12T09:01:00.000Z",
    };
    const updatedIncident: Incident = {
      ...baseIncident,
      status: "resolved",
      updated_at: "2026-04-12T09:10:00.000Z",
    };

    expect(upsertIncident([olderIncident, baseIncident], updatedIncident)).toEqual([
      updatedIncident,
      olderIncident,
    ]);
  });
});

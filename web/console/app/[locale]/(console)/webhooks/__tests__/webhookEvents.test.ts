import { describe, expect, it } from "vitest";
import { eventParts } from "../webhookEvents";

// Risk: the list says a webhook sends fewer (or more) events than the hub sends it.
describe("eventParts", () => {
  it("reads none and all nine as every event, as the hub sends them", () => {
    expect(eventParts([])).toEqual([{ key: "all" }]);
    expect(eventParts([
      "asset.created", "asset.updated", "asset.deleted", "alert.fired", "alert.resolved",
      "incident.created", "incident.resolved", "action.completed", "update.completed",
    ])).toEqual([{ key: "all" }]);
  });

  it("names whole groups once and single events by their label", () => {
    expect(eventParts(["alert.resolved", "alert.fired", "asset.created", "update.completed"])).toEqual([
      { key: "names.asset_created" }, { key: "groups.alerts" }, { key: "names.update_completed" },
    ]);
  });

  it("keeps event names the console has no words for", () => {
    expect(eventParts(["incident.created", "backup.finished"])).toEqual([{ key: "names.incident_created" }, { raw: "backup.finished" }]);
  });
});

import type { IncidentEvent } from "../console/models";
import { ensureRecord } from "./responseGuards";

export type IncidentTimelinePage = { events: IncidentEvent[]; next_cursor?: string };

// Share the wire decoder between the proxy and hook: malformed data is an error,
// never evidence that an incident has no events. Unknown event types remain valid.
export function decodeIncidentTimeline(data: unknown, incidentID: string): IncidentTimelinePage {
  const page = ensureRecord(data);
  if (!page || !Array.isArray(page.events) || page.events.length > 500 ||
      (page.next_cursor !== undefined && (typeof page.next_cursor !== "string" || page.next_cursor.length > 4096))) {
    throw new Error("Invalid incident timeline response");
  }
  const seen = new Set<string>();
  const events = page.events.map((value): IncidentEvent => {
    const row = ensureRecord(value);
    if (!row || typeof row.id !== "string" || !row.id || seen.has(row.id) ||
        row.incident_id !== incidentID || typeof row.event_type !== "string" || !row.event_type ||
        typeof row.source_ref !== "string" || typeof row.summary !== "string" ||
        typeof row.occurred_at !== "string" || !Number.isFinite(Date.parse(row.occurred_at)) ||
        typeof row.created_at !== "string" || !Number.isFinite(Date.parse(row.created_at)) ||
        (row.severity !== undefined && typeof row.severity !== "string") ||
        (row.metadata !== undefined && !ensureRecord(row.metadata))) {
      throw new Error("Invalid incident timeline event");
    }
    seen.add(row.id);
    return row as IncidentEvent;
  });
  if (page.next_cursor && events.length === 0) throw new Error("Empty incident timeline page has a cursor");
  return { events, ...(page.next_cursor ? { next_cursor: page.next_cursor as string } : {}) };
}

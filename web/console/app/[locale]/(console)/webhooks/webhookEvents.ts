// The events a webhook can send (the hub's broadcast names) and how a list of them reads. The hub sends every event
// to a webhook that names none.

export const EVENT_TYPES = [
  "asset.created",
  "asset.updated",
  "asset.deleted",
  "alert.fired",
  "alert.resolved",
  "incident.created",
  "incident.resolved",
  "action.completed",
  "update.completed",
] as const;

export const EVENT_GROUPS = [
  { id: "devices", prefix: "asset." },
  { id: "alerts", prefix: "alert." },
  { id: "issues", prefix: "incident." },
  { id: "actions", prefix: "action." },
  { id: "updates", prefix: "update." },
] as const;

/** An i18n key under webhooks.events, or an event name the console has no words for. */
export type EventPart = { key: string } | { raw: string };

const key = (event: string) => event.replace(".", "_");

/**
 * A webhook's events in words: "All events" for none or all nine, whole groups by their name ("Alerts") when every
 * event of a group with more than one is on, single events by their own label ("Device added"), in the menu's order.
 */
export function eventParts(events: string[]): EventPart[] {
  const on = new Set(events);
  const known = EVENT_TYPES.filter((e) => on.has(e));
  if (on.size === 0 || known.length === EVENT_TYPES.length) return [{ key: "all" }];
  const parts: EventPart[] = [];
  for (const group of EVENT_GROUPS) {
    const members = EVENT_TYPES.filter((e) => e.startsWith(group.prefix));
    const picked = members.filter((e) => on.has(e));
    if (members.length > 1 && picked.length === members.length) parts.push({ key: `groups.${group.id}` });
    else for (const e of picked) parts.push({ key: `names.${key(e)}` });
  }
  for (const e of events) if (!(EVENT_TYPES as readonly string[]).includes(e)) parts.push({ raw: e });
  return parts;
}

/** The i18n key of one event's label (webhooks.events.names.*). */
export const eventKey = (event: (typeof EVENT_TYPES)[number]) => `names.${key(event)}`;

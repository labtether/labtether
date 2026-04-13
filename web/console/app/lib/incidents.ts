import type { Incident } from "../console/models";
import { ensureRecord } from "./responseGuards";

export function extractIncident(data: unknown): Incident | null {
  const payload = ensureRecord(data);
  if (payload?.incident && typeof payload.incident === "object") {
    return payload.incident as Incident;
  }
  if (payload && typeof payload.id === "string") {
    return payload as Incident;
  }
  return null;
}

export function upsertIncident(incidents: Incident[], nextIncident: Incident): Incident[] {
  const remaining = incidents.filter((incident) => incident.id !== nextIncident.id);
  return [nextIncident, ...remaining].sort((left, right) => {
    const leftTime = Date.parse(left.updated_at);
    const rightTime = Date.parse(right.updated_at);
    return rightTime - leftTime;
  });
}

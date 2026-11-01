"use client";

import { useEffect, useRef, useState } from "react";
import type { AlertInstance, Incident, IncidentEvent } from "../../../console/models";

export type AlertIssue = { id: string; title: string };

const MAX_ISSUES = 8;
const LIST_TTL_MS = 60_000;

/** Issues that could hold this alert: open when it started (or still open), the latest first, at most eight. */
export function candidateIssues(incidents: Incident[], startedAt: string): Incident[] {
  const start = Date.parse(startedAt) || 0;
  return incidents
    .filter((issue) => !issue.resolved_at || Date.parse(issue.resolved_at) >= start)
    .sort((a, b) => (Date.parse(b.created_at) || 0) - (Date.parse(a.created_at) || 0))
    .slice(0, MAX_ISSUES);
}

const listOf = <T,>(data: unknown, key: string): T[] => {
  if (Array.isArray(data)) return data as T[];
  const value = data && typeof data === "object" ? (data as Record<string, unknown>)[key] : null;
  return Array.isArray(value) ? (value as T[]) : [];
};

/**
 * The issue this alert belongs to, found the way the hub records it: an alert_fired event with source_ref
 * "alert_instances:<alert id>" on the issue's timeline. Looked up once per alert (a short pause first, so moving
 * through the list with j/k does not fetch for every row); null while unknown or when no issue holds it.
 */
export function useAlertIssue(alert: AlertInstance | null): AlertIssue | null {
  const found = useRef(new Map<string, AlertIssue | null>());
  const issues = useRef<{ at: number; list: Incident[] } | null>(null);
  const [, setVersion] = useState(0);
  const id = alert?.id ?? null;
  const startedAt = alert?.started_at ?? "";

  useEffect(() => {
    if (!id || found.current.has(id)) return;
    const controller = new AbortController();
    const { signal } = controller;
    const timer = setTimeout(async () => {
      try {
        if (!issues.current || Date.now() - issues.current.at > LIST_TTL_MS) {
          const response = await fetch("/api/incidents", { cache: "no-store", signal });
          if (!response.ok) return;
          issues.current = { at: Date.now(), list: listOf<Incident>(await response.json(), "incidents") };
        }
        let match: AlertIssue | null = null;
        for (const issue of candidateIssues(issues.current.list, startedAt)) {
          const response = await fetch(`/api/incidents/${encodeURIComponent(issue.id)}/events?limit=100`, { cache: "no-store", signal });
          if (!response.ok) continue;
          const events = listOf<IncidentEvent>(await response.json(), "events");
          if (events.some((event) => event.source_ref === `alert_instances:${id}`)) {
            match = { id: issue.id, title: issue.title };
            break;
          }
        }
        found.current.set(id, match);
        setVersion((v) => v + 1);
      } catch {
        // Aborted or unavailable: the alert keeps the plain link to Issues.
      }
    }, 300);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [id, startedAt]);

  return id ? found.current.get(id) ?? null : null;
}

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { AuditEntry } from "./auditModel";

export const AUDIT_PAGE = 200;
/** Export reads at most this many rows; the Hub keeps 180 days by default. */
export const AUDIT_EXPORT_MAX = 20_000;

type Page = { events: AuditEntry[]; full: boolean };

async function fetchPage(offset: number, limit = AUDIT_PAGE): Promise<Page> {
  const response = await fetch(`/api/v2/audit/events?limit=${limit}&offset=${offset}`, { cache: "no-store" });
  if (!response.ok) throw new Error(String(response.status));
  const payload = (await response.json()) as { data?: { events?: AuditEntry[] } };
  const events = Array.isArray(payload.data?.events) ? payload.data.events : [];
  return { events, full: events.length >= limit };
}

/** Newer rows first, each id once. */
function merge(newer: AuditEntry[], older: AuditEntry[]): AuditEntry[] {
  const seen = new Set<string>();
  return [...newer, ...older].filter((e) => (seen.has(e.id) ? false : (seen.add(e.id), true)));
}

/**
 * The whole audit log a page at a time, for the owner and admins: the newest page (read again on refresh), "Load
 * more" for older pages, and every row for an export. Off (enabled false) for everyone else, whose own rows come
 * with the status.
 */
export function useAuditPages(enabled: boolean) {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [more, setMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const loaded = useRef(0);

  const refresh = useCallback(async () => {
    if (!enabled) return;
    try {
      const first = await fetchPage(0);
      setEntries((current) => merge(first.events, current));
      if (loaded.current === 0) {
        loaded.current = first.events.length;
        setMore(first.full);
      }
      setFailed(false);
    } catch {
      setFailed(true);
    }
  }, [enabled]);

  useEffect(() => {
    if (!enabled) return;
    setLoading(true);
    void refresh().finally(() => setLoading(false));
  }, [enabled, refresh]);

  const loadMore = useCallback(async () => {
    setLoading(true);
    try {
      const page = await fetchPage(loaded.current);
      loaded.current += page.events.length;
      setEntries((current) => merge(current, page.events));
      setMore(page.full);
      setFailed(false);
    } catch {
      setFailed(true);
    } finally {
      setLoading(false);
    }
  }, []);

  /** Every row the Hub has (up to AUDIT_EXPORT_MAX), for the CSV export. */
  const readAll = useCallback(async (): Promise<AuditEntry[]> => {
    let all: AuditEntry[] = [];
    for (let offset = 0; offset < AUDIT_EXPORT_MAX; offset += 1000) {
      const page = await fetchPage(offset, 1000);
      all = merge(all, page.events);
      if (!page.full) break;
    }
    return all;
  }, []);

  return { entries, more, loading, failed, refresh, loadMore, readAll };
}

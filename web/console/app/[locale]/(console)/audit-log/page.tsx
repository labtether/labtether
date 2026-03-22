"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { Download, ScrollText } from "lucide-react";
import { PageHeader } from "../../../components/PageHeader";
import { Card } from "../../../components/ui/Card";
import { Button } from "../../../components/ui/Button";
import { Input } from "../../../components/ui/Input";
import { EmptyState } from "../../../components/ui/EmptyState";
import { useSlowStatus, useStatusControls } from "../../../contexts/StatusContext";
import { downloadCSV } from "../../../lib/export";
import type { AuditEvent } from "../../../console/models";

// ── Helpers ──

function formatTimestamp(ts: string): string {
  try {
    return new Intl.DateTimeFormat(undefined, {
      year: "numeric",
      month: "short",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    }).format(new Date(ts));
  } catch {
    return ts;
  }
}

function truncate(str: string | undefined, max: number): string {
  if (!str) return "—";
  return str.length > max ? `${str.slice(0, max)}…` : str;
}

// ── Sub-components ──

function DecisionBadge({ decision, allow, deny }: { decision?: string; allow: string; deny: string }) {
  if (!decision) return <span className="text-[var(--muted)] text-xs">—</span>;
  const lower = decision.toLowerCase();
  if (lower === "allow") {
    return (
      <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium bg-[var(--ok-glow)] text-[var(--ok)]">
        {allow}
      </span>
    );
  }
  if (lower === "deny") {
    return (
      <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium bg-[var(--bad-glow)] text-[var(--bad)]">
        {deny}
      </span>
    );
  }
  return (
    <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium bg-[var(--surface)] text-[var(--muted)] border border-[var(--line)]">
      {decision}
    </span>
  );
}

function TypeBadge({ type }: { type: string }) {
  return (
    <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium bg-[var(--surface)] text-[var(--muted)] border border-[var(--line)] font-mono">
      {type}
    </span>
  );
}

// ── Row with expandable details ──

type AuditEventWithDetails = AuditEvent & {
  actor_id?: string;
  details?: Record<string, unknown>;
  reason?: string;
};

function AuditRow({
  event,
  allow,
  deny,
  detailsLabel,
}: {
  event: AuditEventWithDetails;
  allow: string;
  deny: string;
  detailsLabel: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const hasDetails = event.details != null || event.reason;

  return (
    <>
      <tr
        className={`border-b border-[var(--line)] transition-colors ${hasDetails ? "cursor-pointer hover:bg-[var(--hover)]" : ""}`}
        style={{ transitionDuration: "var(--dur-instant)" }}
        onClick={hasDetails ? () => setExpanded((v) => !v) : undefined}
      >
        <td className="px-3 py-2 text-xs text-[var(--muted)] font-mono whitespace-nowrap">
          {formatTimestamp(event.timestamp)}
        </td>
        <td className="px-3 py-2">
          <TypeBadge type={event.type} />
        </td>
        <td className="px-3 py-2 text-xs text-[var(--text)] font-mono">
          {truncate(event.actor_id, 24)}
        </td>
        <td className="px-3 py-2 text-xs text-[var(--muted)]">
          {truncate(event.target, 32)}
        </td>
        <td className="px-3 py-2">
          <DecisionBadge decision={event.decision} allow={allow} deny={deny} />
        </td>
        <td className="px-3 py-2 text-xs text-[var(--muted)]">
          {hasDetails ? (
            <span className="text-[var(--accent)] hover:underline">
              {expanded ? "▲" : "▼"} {detailsLabel}
            </span>
          ) : "—"}
        </td>
      </tr>
      {expanded && hasDetails ? (
        <tr className="border-b border-[var(--line)] bg-[var(--surface)]">
          <td colSpan={6} className="px-4 py-3">
            <pre className="text-[10px] font-mono text-[var(--text)] whitespace-pre-wrap break-all overflow-x-auto max-h-64 overflow-y-auto">
              {JSON.stringify(
                { reason: event.reason ?? undefined, ...event.details },
                null,
                2,
              )}
            </pre>
          </td>
        </tr>
      ) : null}
    </>
  );
}

// ── Page ──

const AUTO_REFRESH_MS = 30_000;

export default function AuditLogPage() {
  const t = useTranslations("audit-log");
  const slowStatus = useSlowStatus();
  const { fetchStatus } = useStatusControls();

  const allEvents = useMemo(
    () => (slowStatus?.recentAudit ?? []) as AuditEventWithDetails[],
    [slowStatus?.recentAudit],
  );

  // ── Collect unique event types for filter dropdown ──
  const eventTypes = useMemo(() => {
    const types = new Set<string>();
    for (const e of allEvents) types.add(e.type);
    return Array.from(types).sort();
  }, [allEvents]);

  // ── Filter state ──
  const [search, setSearch] = useState("");
  const [typeFilter, setTypeFilter] = useState("");

  // ── Filtered events ──
  const filtered = useMemo(() => {
    let result = allEvents;
    if (typeFilter) {
      result = result.filter((e) => e.type === typeFilter);
    }
    if (search.trim()) {
      const q = search.trim().toLowerCase();
      result = result.filter(
        (e) =>
          e.type.toLowerCase().includes(q) ||
          (e.actor_id ?? "").toLowerCase().includes(q) ||
          (e.target ?? "").toLowerCase().includes(q) ||
          (e.decision ?? "").toLowerCase().includes(q),
      );
    }
    // Most recent first
    return [...result].sort(
      (a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime(),
    );
  }, [allEvents, search, typeFilter]);

  // ── Auto-refresh every 30 s ──
  useEffect(() => {
    const id = setInterval(() => {
      void fetchStatus();
    }, AUTO_REFRESH_MS);
    return () => clearInterval(id);
  }, [fetchStatus]);

  // ── CSV export ──
  const handleExport = useCallback(() => {
    const rows = filtered.map((e) => ({
      timestamp: e.timestamp,
      type: e.type,
      actor_id: e.actor_id ?? "",
      target: e.target ?? "",
      decision: e.decision ?? "",
      details: e.details ? JSON.stringify(e.details) : "",
      reason: (e as AuditEventWithDetails).reason ?? "",
    }));
    downloadCSV(rows, `audit-log-${new Date().toISOString().slice(0, 10)}.csv`);
  }, [filtered]);

  return (
    <>
      <PageHeader
        title={t("title")}
        subtitle={t("subtitle")}
        action={(
          <Button
            variant="ghost"
            size="sm"
            onClick={handleExport}
            disabled={filtered.length === 0}
          >
            <Download size={14} />
            {t("exportCSV")}
          </Button>
        )}
      />

      <Card variant="flush">
        {/* Toolbar */}
        <div className="flex flex-wrap items-center gap-2 px-3 py-2 border-b border-[var(--line)]">
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("search")}
            className="max-w-[260px] h-7 text-xs"
          />
          <select
            value={typeFilter}
            onChange={(e) => setTypeFilter(e.target.value)}
            className="h-7 rounded-md border border-[var(--line)] bg-[var(--surface)] text-xs text-[var(--text)] px-2 focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
          >
            <option value="">{t("allTypes")}</option>
            {eventTypes.map((type) => (
              <option key={type} value={type}>{type}</option>
            ))}
          </select>
        </div>

        {/* Table or empty state */}
        {filtered.length === 0 ? (
          <div className="p-4">
            <EmptyState
              icon={ScrollText}
              title={t("noEvents")}
              description={t("noEventsDesc")}
            />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="px-3 py-2 text-[10px] font-semibold text-[var(--muted)] uppercase tracking-wide whitespace-nowrap">
                    {t("timestamp")}
                  </th>
                  <th className="px-3 py-2 text-[10px] font-semibold text-[var(--muted)] uppercase tracking-wide">
                    {t("type")}
                  </th>
                  <th className="px-3 py-2 text-[10px] font-semibold text-[var(--muted)] uppercase tracking-wide">
                    {t("actor")}
                  </th>
                  <th className="px-3 py-2 text-[10px] font-semibold text-[var(--muted)] uppercase tracking-wide">
                    {t("target")}
                  </th>
                  <th className="px-3 py-2 text-[10px] font-semibold text-[var(--muted)] uppercase tracking-wide">
                    {t("decision")}
                  </th>
                  <th className="px-3 py-2 text-[10px] font-semibold text-[var(--muted)] uppercase tracking-wide">
                    {t("details")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((event) => (
                  <AuditRow
                    key={event.id}
                    event={event}
                    allow={t("allow")}
                    deny={t("deny")}
                    detailsLabel={t("details")}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </>
  );
}

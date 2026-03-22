"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Bell, RefreshCw } from "lucide-react";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { EmptyState } from "../../../components/ui/EmptyState";
import { SegmentedTabs } from "../../../components/ui/SegmentedTabs";
import { apiFetch } from "../../../lib/api";

// ── Types ──

type NotificationStatus = "sent" | "failed" | "pending";
type StatusFilter = "all" | NotificationStatus;

type NotificationRecord = {
  id: string;
  channel_id: string;
  alert_instance_id: string;
  route_id: string;
  status: NotificationStatus;
  sent_at: string | null;
  error: string;
  retry_count: number;
  max_retries: number;
  next_retry_at: string | null;
  created_at: string;
};

type HistoryPayload = {
  history: NotificationRecord[];
};

// ── Status badge ──

const STATUS_STYLES: Record<NotificationStatus, { dot: string; bg: string; text: string }> = {
  sent: {
    dot: "bg-[var(--ok)]",
    bg: "bg-[var(--ok-glow)]",
    text: "text-[var(--ok)]",
  },
  failed: {
    dot: "bg-[var(--bad)]",
    bg: "bg-[var(--bad-glow)]",
    text: "text-[var(--bad)]",
  },
  pending: {
    dot: "bg-[var(--warn)]",
    bg: "bg-[var(--warn-glow)]",
    text: "text-[var(--warn)]",
  },
};

function StatusBadge({ status }: { status: string }) {
  const normalized = status.toLowerCase() as NotificationStatus;
  const styles = STATUS_STYLES[normalized] ?? STATUS_STYLES.pending;
  const label = status.charAt(0).toUpperCase() + status.slice(1);

  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-lg px-2 py-0.5 text-xs font-medium ${styles.bg} ${styles.text}`}
    >
      <span className={`inline-block rounded-full h-1.5 w-1.5 ${styles.dot}`} />
      {label}
    </span>
  );
}

// ── Filter options ──

const STATUS_FILTER_OPTIONS: { id: StatusFilter; labelKey: string }[] = [
  { id: "all", labelKey: "all" },
  { id: "sent", labelKey: "sent" },
  { id: "failed", labelKey: "failed" },
  { id: "pending", labelKey: "pending" },
];

const AUTO_REFRESH_MS = 30_000;

// ── Component ──

export function AlertDeliveryLogTab() {
  const t = useTranslations("notification-center");

  const [records, setRecords] = useState<NotificationRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");

  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchHistory = useCallback(async (isManual = false) => {
    if (isManual) setRefreshing(true);
    try {
      const { response, data } = await apiFetch<HistoryPayload>(
        "/api/notifications/history?limit=200",
      );
      if (!response.ok) {
        const errPayload = data as { error?: string } | null;
        setError(errPayload?.error ?? `Failed to load notification history (${response.status})`);
        return;
      }
      setError(null);
      setRecords(data?.history ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load notification history");
    } finally {
      setLoading(false);
      if (isManual) setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void fetchHistory();
    intervalRef.current = setInterval(() => { void fetchHistory(); }, AUTO_REFRESH_MS);
    return () => {
      if (intervalRef.current !== null) clearInterval(intervalRef.current);
    };
  }, [fetchHistory]);

  const filtered =
    statusFilter === "all"
      ? records
      : records.filter((r) => r.status === statusFilter);

  const tabOptions = STATUS_FILTER_OPTIONS.map((opt) => ({
    id: opt.id,
    label: t(opt.labelKey),
  }));

  return (
    <Card variant="flush">
      {/* Toolbar: filter tabs + refresh */}
      <div className="flex items-center justify-between gap-2 px-3 py-2 border-b border-[var(--line)]">
        <SegmentedTabs
          value={statusFilter}
          options={tabOptions}
          onChange={setStatusFilter}
          size="sm"
        />
        <Button
          variant="ghost"
          size="sm"
          onClick={() => { void fetchHistory(true); }}
          disabled={refreshing}
        >
          <RefreshCw size={13} className={refreshing ? "animate-spin" : ""} />
          {t("refresh")}
        </Button>
      </div>

      {/* Content area */}
      {loading ? (
        <div className="flex items-center justify-center py-16">
          <span className="text-xs text-[var(--muted)] animate-pulse">Loading...</span>
        </div>
      ) : error ? (
        <div className="p-6">
          <p className="text-xs text-[var(--bad)]">{error}</p>
        </div>
      ) : filtered.length === 0 ? (
        <div className="p-4">
          <EmptyState
            icon={Bell}
            title={t("noNotifications")}
            description={t("noNotificationsDesc")}
          />
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="px-4 py-2.5 text-left font-medium text-[var(--muted)] whitespace-nowrap">
                  {t("sentAt")}
                </th>
                <th className="px-4 py-2.5 text-left font-medium text-[var(--muted)] whitespace-nowrap">
                  {t("status")}
                </th>
                <th className="px-4 py-2.5 text-left font-medium text-[var(--muted)] whitespace-nowrap">
                  {t("channel")}
                </th>
                <th className="px-4 py-2.5 text-left font-medium text-[var(--muted)] whitespace-nowrap">
                  {t("alert")}
                </th>
                <th className="px-4 py-2.5 text-left font-medium text-[var(--muted)] whitespace-nowrap">
                  {t("retries")}
                </th>
                <th className="px-4 py-2.5 text-left font-medium text-[var(--muted)] whitespace-nowrap">
                  {t("error")}
                </th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((record, idx) => {
                const timestamp = record.sent_at ?? record.created_at;
                const displayTime = timestamp
                  ? new Date(timestamp).toLocaleString()
                  : "—";
                const retryLabel =
                  record.max_retries > 0
                    ? `${record.retry_count} / ${record.max_retries}`
                    : String(record.retry_count);
                const errorText = record.error || "—";
                const isEven = idx % 2 === 0;

                return (
                  <tr
                    key={record.id}
                    className={`border-b border-[var(--line)] last:border-0 transition-colors duration-[var(--dur-instant)] hover:bg-[var(--hover)] ${
                      isEven ? "" : "bg-[var(--surface)]/30"
                    }`}
                  >
                    <td className="px-4 py-2.5 font-mono text-[var(--muted)] whitespace-nowrap">
                      {displayTime}
                    </td>
                    <td className="px-4 py-2.5 whitespace-nowrap">
                      <StatusBadge status={record.status} />
                    </td>
                    <td className="px-4 py-2.5 text-[var(--text)] whitespace-nowrap font-mono">
                      {record.channel_id || "—"}
                    </td>
                    <td className="px-4 py-2.5 text-[var(--muted)] whitespace-nowrap font-mono">
                      {record.alert_instance_id || "—"}
                    </td>
                    <td className="px-4 py-2.5 text-[var(--muted)] tabular-nums">
                      {retryLabel}
                    </td>
                    <td className="px-4 py-2.5 max-w-[18rem]">
                      <span
                        className="block truncate text-[var(--bad)] empty:text-[var(--muted)]"
                        title={record.error || undefined}
                      >
                        {errorText}
                      </span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}

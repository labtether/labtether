"use client";

import { useLocale, useTranslations } from "next-intl";
import type { StorageInsightEvent, StorageRow } from "./storageOperationsModel";
import { isHealthyHealth } from "./storageOperationsUtils";
import { DiKV, DiLog } from "./proxmox/DiList";
import { sizeText, whenText } from "./proxmox/diFormat";

type StoragePoolExpandedDetailsProps = {
  row: StorageRow;
  rowPoolEvents: StorageInsightEvent[];
  expandedTaskRef: string | null;
  taskLogLoadingRef: string | null;
  taskLogs: Record<string, string>;
  onOpenTaskLog: (node: string, upid: string) => Promise<void> | void;
};

/** Days as words: "under a day", "12 days", "over a year"; null when there is no forecast. */
export function useDaysWords() {
  const t = useTranslations("devices-proxmox.pools");
  return (days: number | null): string | null => {
    if (days == null) return null;
    if (days < 1) return t("underDay");
    if (days > 365) return t("overYear");
    return t("days", { count: Math.round(days) });
  };
}

/** The panel under an open pool: health, free space, forecast, snapshots, guests, then its recent events. */
export function StoragePoolExpandedDetails({
  row,
  rowPoolEvents,
  expandedTaskRef,
  taskLogLoadingRef,
  taskLogs,
  onOpenTaskLog,
}: StoragePoolExpandedDetailsProps) {
  const t = useTranslations("devices-proxmox.pools");
  const locale = useLocale();
  const daysWords = useDaysWords();
  const growth = row.growthBytes7d == null ? null : row.growthBytes7d === 0 ? "0 B" : `${row.growthBytes7d > 0 ? "+" : "−"}${sizeText(Math.abs(row.growthBytes7d))}`;
  const guests = [
    row.vmCount > 0 ? t("vms", { count: row.vmCount }) : null,
    row.ctCount > 0 ? t("cts", { count: row.ctCount }) : null,
  ].filter(Boolean).join(", ");

  return (
    <div className="di-panel di-so-panel">
      <DiKV
        columns={2}
        items={[
          [t("health"), <span key="h" className={isHealthyHealth(row.health) ? "" : "di-bad"}>{row.health}</span>],
          [t("free"), sizeText(row.freeBytes) || null],
          [t("growth"), growth],
          [t("to80"), daysWords(row.daysTo80)],
          [t("toFull"), daysWords(row.daysToFull)],
          [t("confidence"), row.daysToFull != null || row.daysTo80 != null ? t(`confidenceLevels.${row.confidence}`) : null],
          [t("fragmentation"), row.fragPercent != null ? `${row.fragPercent}%` : null],
          [t("dedup"), row.dedupRatio != null ? `${row.dedupRatio.toFixed(2)}×` : null],
          [t("snapshots"), row.snapshotCount > 0 ? t("snapshotsValue", { count: row.snapshotCount, size: sizeText(row.snapshotBytes) }) : null],
          [t("guests"), guests || t("noGuests")],
          [t("scrub"), row.scrubOverdue ? <span key="s" className="di-warn">{t("scrubOverdue")}</span> : null],
          [t("data"), row.stale ? <span key="d" className="di-warn">{t("staleData")}</span> : null],
        ]}
      />
      <div className="di-so-events">
        <b>{t("recentEvents")}</b>
        {rowPoolEvents.length === 0 ? <span className="di-grey">{t("noPoolEvents")}</span> : null}
        {rowPoolEvents.slice(0, 4).map((event, index) => {
          const ref = event.node && event.upid ? `${event.node}/${event.upid}` : null;
          return (
            <div key={`${event.timestamp ?? "event"}-${event.upid ?? index}`}>
              <div className="di-so-event">
                <span className="di-time di-left">{whenText(event.timestamp, locale)}</span>
                <span className="di-so-msg">{event.message}</span>
                {ref ? (
                  <button type="button" className="di-so-link" onClick={() => void onOpenTaskLog(event.node!, event.upid!)}>
                    {expandedTaskRef === ref ? t("hideLog") : t("log")}
                  </button>
                ) : null}
              </div>
              {ref && expandedTaskRef === ref ? (
                <DiLog>{taskLogLoadingRef === ref ? t("loadingLog") : (taskLogs[ref] ?? "")}</DiLog>
              ) : null}
            </div>
          );
        })}
      </div>
      {row.reason ? <p className="di-so-reason">{row.reason}</p> : null}
    </div>
  );
}

"use client";

import { useTranslations } from "next-intl";
import type { StorageInsightEvent, StorageRow } from "./storageOperationsModel";
import { StorageOperationsPoolRow } from "./StorageOperationsPoolRow";
import { StoragePoolExpandedDetails } from "./StoragePoolExpandedDetails";
import type { DiMenuItem } from "./proxmox/DiControls";
import { DiCols, DiEmpty, DiFoot, DiList } from "./proxmox/DiList";

type StorageOperationsTableCardProps = {
  loading: boolean;
  rows: StorageRow[];
  filteredRows: StorageRow[];
  poolEvents: Map<string, StorageInsightEvent[]>;
  expandedRowKey: string | null;
  onExpandedRowChange: (nextKey: string | null) => void;
  onClearFilter: () => void;
  onOpenTaskLog: (node: string, upid: string) => Promise<void> | void;
  expandedTaskRef: string | null;
  taskLogLoadingRef: string | null;
  taskLogs: Record<string, string>;
  menuFor: (row: StorageRow) => DiMenuItem[];
};

/** The host's pools, riskiest first; a row opens its details and its ⋯ holds backups of the guests on it. */
export function StorageOperationsTableCard({
  loading,
  rows,
  filteredRows,
  poolEvents,
  expandedRowKey,
  onExpandedRowChange,
  onClearFilter,
  onOpenTaskLog,
  expandedTaskRef,
  taskLogLoadingRef,
  taskLogs,
  menuFor,
}: StorageOperationsTableCardProps) {
  const t = useTranslations("devices-proxmox.pools");
  const kit = useTranslations("devices-proxmox.kit");

  return (
    <DiList
      cols="minmax(0, 1fr) minmax(48px, 140px) 48px 112px 28px"
      colsM="minmax(0, 1fr) 48px 112px 28px"
      colsS="minmax(0, 1fr) 48px 28px"
      label={t("pools")}
    >
      <DiCols>
        <span>{t("pool")}</span>
        <span className="di-m">{t("used")}</span>
        <span />
        <span className="di-s" />
        <span />
      </DiCols>
      {filteredRows.length === 0 ? (
        <DiEmpty title={loading && rows.length === 0 ? kit("loading") : rows.length === 0 ? t("none") : t("noMatch")} />
      ) : null}
      {filteredRows.map((row) => {
        const isExpanded = expandedRowKey === row.key;
        return (
          <StorageOperationsPoolRow
            key={row.key}
            row={row}
            isExpanded={isExpanded}
            onToggleExpanded={() => onExpandedRowChange(isExpanded ? null : row.key)}
            menu={menuFor(row)}
            details={
              <StoragePoolExpandedDetails
                row={row}
                rowPoolEvents={poolEvents.get(row.key) ?? []}
                expandedTaskRef={expandedTaskRef}
                taskLogLoadingRef={taskLogLoadingRef}
                taskLogs={taskLogs}
                onOpenTaskLog={onOpenTaskLog}
              />
            }
          />
        );
      })}
      {filteredRows.length !== rows.length ? (
        <DiFoot
          left={kit("shown", { shown: filteredRows.length, total: rows.length })}
          right={<button type="button" onClick={onClearFilter}>{kit("clearFilter")}</button>}
        />
      ) : null}
    </DiList>
  );
}

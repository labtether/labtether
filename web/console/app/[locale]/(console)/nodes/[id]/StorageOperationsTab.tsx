"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";

import { Button } from "../../../../components/ui/Button";
import type { Asset } from "../../../../console/models";
import type { ProxmoxStorageDetails, RiskFilter, StorageRow } from "./storageOperationsModel";
import { taskStatusBadge } from "./storageOperationsUtils";
import { StorageOperationsTableCard } from "./StorageOperationsTableCard";
import { useStorageOperationsActions } from "./useStorageOperationsActions";
import { useStorageOperationsData } from "./useStorageOperationsData";
import { DiPick, type DiMenuItem } from "./proxmox/DiControls";
import { DiCols, DiDots, DiEmpty, DiList, DiLog, DiMain, DiNote, DiPage, DiRow, DiSection, DiTime, type DiTone } from "./proxmox/DiList";
import { whenText } from "./proxmox/diFormat";

type StorageOperationsTabProps = {
  hostAsset: Asset;
  proxmoxDetails: ProxmoxStorageDetails | null;
  proxmoxLoading: boolean;
  proxmoxError: string | null;
  onRetry: () => void;
  onRunProxmoxAction?: (
    actionID: string,
    target: string,
    params?: Record<string, string>,
  ) => Promise<void> | void;
  proxmoxActionRunning?: boolean;
  proxmoxActionMessage?: string | null;
  proxmoxActionError?: string | null;
  onOpenWorkloads?: () => void;
};

const BACKUP_TARGETS = 3;

function severityTone(severity?: string): DiTone {
  const value = (severity ?? "").toLowerCase();
  if (value === "critical") return "bad";
  if (value === "warning") return "warn";
  return "";
}

/** A Proxmox host's storage: its pools by risk (use, forecast, health) and the storage tasks of the last 24 hours. */
export function StorageOperationsTab({
  hostAsset,
  proxmoxDetails,
  proxmoxLoading,
  proxmoxError,
  onRetry,
  onRunProxmoxAction,
  proxmoxActionRunning = false,
  proxmoxActionMessage = null,
  proxmoxActionError = null,
  onOpenWorkloads,
}: StorageOperationsTabProps) {
  const t = useTranslations("devices-proxmox.pools");
  const kit = useTranslations("devices-proxmox.kit");
  const locale = useLocale();
  const [expandedRowKey, setExpandedRowKey] = useState<string | null>(null);

  const {
    riskFilter, setRiskFilter, insightsLoading, insightsError, fetchInsights, storageNode,
    proxmoxCollectorID, rows, timelineEvents, poolEvents, filteredRows, riskChips,
  } = useStorageOperationsData({ hostAsset, proxmoxDetails });

  const {
    expandedTaskRef, taskLogLoadingRef, taskLogs, taskLogError, localActionError, openTaskLog, runPoolBackup,
  } = useStorageOperationsActions({ fetchInsights, onRunProxmoxAction, proxmoxCollectorID, storageNode });

  const problem = proxmoxError ? t("proxmoxFailed") : proxmoxActionError ?? localActionError ?? taskLogError;
  const filters = riskChips.filter((chip) => chip.key === "all" || chip.value > 0 || chip.key === riskFilter);
  const updated = whenText(proxmoxDetails?.fetched_at, locale);

  const menuFor = (row: StorageRow): DiMenuItem[] => {
    const canBackup = !!onRunProxmoxAction && !proxmoxActionRunning;
    const backups: DiMenuItem[] = [
      ...row.vmIDs.slice(0, BACKUP_TARGETS).map((vmid) => ({
        label: t("backupVm", { vmid }),
        disabled: !canBackup,
        onSelect: () => void runPoolBackup(row.poolName, "vm", vmid),
      })),
      ...row.ctIDs.slice(0, BACKUP_TARGETS).map((vmid) => ({
        label: t("backupCt", { vmid }),
        disabled: !canBackup,
        onSelect: () => void runPoolBackup(row.poolName, "ct", vmid),
      })),
    ];
    const latest = (poolEvents.get(row.key) ?? []).find((event) => event.node?.trim() && event.upid?.trim());
    const more: DiMenuItem[] = [
      { label: expandedRowKey === row.key ? t("closeDetails") : t("openDetails"), onSelect: () => setExpandedRowKey(expandedRowKey === row.key ? null : row.key) },
      ...(latest ? [{ label: t("latestLog"), onSelect: () => { setExpandedRowKey(row.key); void openTaskLog(latest.node!, latest.upid!); } }] : []),
      ...(onOpenWorkloads ? [{ label: t("openGuests"), onSelect: onOpenWorkloads }] : []),
    ];
    return backups.length ? [...backups, { separator: true }, ...more] : more;
  };

  return (
    <DiPage className="di-so">
      <DiSection
        title={t("pools")}
        right={
          <>
            {updated ? <span>{t("updated", { when: updated })}</span> : null}
            {filters.length > 1 ? (
              <DiPick
                label={t("filterLabel")}
                options={filters.map((chip) => ({ id: chip.key, label: t(`filters.${chip.key}`) }))}
                value={riskFilter}
                onChange={(next: RiskFilter) => setRiskFilter(next)}
              />
            ) : null}
            <Button size="sm" onClick={() => { onRetry(); void fetchInsights(); }}>{kit("refresh")}</Button>
          </>
        }
      >
        {problem ? <DiNote tone="bad">{problem}</DiNote> : proxmoxActionMessage ? <DiNote>{proxmoxActionMessage}</DiNote> : null}
        {insightsError && rows.length > 0 ? <DiNote tone="warn">{t("insightsFailed")}</DiNote> : null}
        <StorageOperationsTableCard
          loading={proxmoxLoading || insightsLoading}
          rows={rows}
          filteredRows={filteredRows}
          poolEvents={poolEvents}
          expandedRowKey={expandedRowKey}
          onExpandedRowChange={setExpandedRowKey}
          onClearFilter={() => setRiskFilter("all")}
          onOpenTaskLog={openTaskLog}
          expandedTaskRef={expandedTaskRef}
          taskLogLoadingRef={taskLogLoadingRef}
          taskLogs={taskLogs}
          menuFor={menuFor}
        />
      </DiSection>

      <DiSection title={t("activity")}>
        <DiList cols="64px minmax(0, 1fr) 80px" colsS="56px minmax(0, 1fr) auto" label={t("activity")}>
          <DiCols>
            <span>{t("time")}</span>
            <span>{t("event")}</span>
            <span>{t("state")}</span>
          </DiCols>
          {timelineEvents.length === 0 ? <DiEmpty title={t("noEvents")} /> : null}
          {timelineEvents.slice(0, 20).map((event, index) => {
            const ref = event.node && event.upid ? `${event.node}/${event.upid}` : null;
            const open = ref != null && expandedTaskRef === ref;
            const state = event.task_status || event.exit_status ? taskStatusBadge(event.task_status, event.exit_status) : "";
            return (
              <DiRow
                key={`${event.timestamp ?? "event"}-${event.upid ?? index}-${index}`}
                tone={state === "pending" ? "busy" : state === "bad" ? "bad" : severityTone(event.severity)}
                onOpen={ref ? () => void openTaskLog(event.node!, event.upid!) : undefined}
                selected={open}
                after={open && ref ? <DiLog>{taskLogLoadingRef === ref ? kit("loadingLog") : (taskLogs[ref] ?? "")}</DiLog> : null}
              >
                <DiTime className="di-left">{whenText(event.timestamp, locale)}</DiTime>
                <DiMain name={event.message} plain grey={<DiDots parts={[event.pool || null, event.task_type || null]} />} />
                <span className={`di-word${state === "bad" ? " bad" : ""}`}>
                  {state === "pending" ? t("running") : state === "bad" ? t("failed") : state === "ok" ? t("ok") : ""}
                </span>
              </DiRow>
            );
          })}
        </DiList>
      </DiSection>
    </DiPage>
  );
}

"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import type {
  ClusterStatusEntry,
  NetworkInterface,
  ProxmoxDetails,
} from "./nodeDetailTypes";
import { DiSeg } from "./proxmox/DiControls";
import { DiEmpty, DiNote, DiPage, DiSheet } from "./proxmox/DiList";
import { ProxmoxBackupTab } from "./proxmox/ProxmoxBackupTab";
import { ProxmoxCephTab } from "./proxmox/ProxmoxCephTab";
import { ProxmoxCertificatesTab } from "./proxmox/ProxmoxCertificatesTab";
import { ProxmoxClusterTab } from "./proxmox/ProxmoxClusterTab";
import { ProxmoxConsoleTab } from "./proxmox/ProxmoxConsoleTab";
import { ProxmoxFirewallTab } from "./proxmox/ProxmoxFirewallTab";
import { ProxmoxHATab } from "./proxmox/ProxmoxHATab";
import { ProxmoxLogsTab } from "./proxmox/ProxmoxLogsTab";
import { ProxmoxMetricsTab } from "./proxmox/ProxmoxMetricsTab";
import { ProxmoxNetworkTab } from "./proxmox/ProxmoxNetworkTab";
import { ProxmoxOverviewTab } from "./proxmox/ProxmoxOverviewTab";
import { ProxmoxReplicationTab } from "./proxmox/ProxmoxReplicationTab";
import { ProxmoxSnapshotsTab } from "./proxmox/ProxmoxSnapshotsTab";
import { ProxmoxStorageTab } from "./proxmox/ProxmoxStorageTab";
import { ProxmoxTasksTab } from "./proxmox/ProxmoxTasksTab";
import { ProxmoxUpdatesTab } from "./proxmox/ProxmoxUpdatesTab";
import { proxmoxViewsFor, type ProxmoxView } from "./proxmox/proxmoxViews";

type ProxmoxDetailsTabProps = {
  proxmoxDetails: ProxmoxDetails | null;
  proxmoxLoading: boolean;
  proxmoxError: string | null;
  proxmoxNode: string;
  proxmoxVMID: string;
  effectiveKind: string;
  canRunProxmoxQuickActions: boolean;
  proxmoxActionRunning: boolean;
  clusterStatus: ClusterStatusEntry[];
  networkInterfaces: NetworkInterface[];
  proxmoxCollectorID: string;
  nodeId: string;
  onRetry: () => void;
  onRunProxmoxQuickAction: (actionID: string, params?: Record<string, string>) => void;
  /** The result of the last snapshot action, when the page passes it. */
  proxmoxActionMessage?: string | null;
  proxmoxActionError?: string | null;
};

export function ProxmoxDetailsTab({
  proxmoxDetails,
  proxmoxLoading,
  proxmoxError,
  proxmoxNode,
  proxmoxVMID,
  effectiveKind,
  canRunProxmoxQuickActions,
  proxmoxActionRunning,
  clusterStatus,
  networkInterfaces,
  proxmoxCollectorID,
  nodeId,
  onRetry,
  onRunProxmoxQuickAction,
  proxmoxActionMessage = null,
  proxmoxActionError = null,
}: ProxmoxDetailsTabProps) {
  const t = useTranslations("devices-proxmox.pve");
  const [chosen, setChosen] = useState<ProxmoxView>("overview");
  const views = useMemo(() => proxmoxViewsFor(effectiveKind, proxmoxDetails), [effectiveKind, proxmoxDetails]);
  const view: ProxmoxView = views.includes(chosen) ? chosen : "overview";

  if (!proxmoxDetails) {
    return (
      <DiPage>
        <DiSheet>
          {proxmoxLoading ? (
            <DiEmpty title={t("loading")} />
          ) : proxmoxError ? (
            <DiEmpty
              title={t("loadFailed")}
              line={proxmoxError}
              action={<Button size="sm" onClick={onRetry}>{t("retry")}</Button>}
            />
          ) : (
            <DiEmpty title={t("noDetails")} line={t("noDetailsLine")} />
          )}
        </DiSheet>
      </DiPage>
    );
  }

  const node = proxmoxNode || proxmoxDetails.node || "";

  return (
    <DiPage className="di-pve">
      <div className="di-seg-row">
        <DiSeg
          label={t("viewsLabel")}
          options={views.map((id) => ({ id, label: t(`views.${id}`) }))}
          value={view}
          onChange={setChosen}
        />
      </div>

      {view === "snapshots" && (proxmoxActionError || proxmoxActionMessage) ? (
        <DiNote tone={proxmoxActionError ? "bad" : ""}>{proxmoxActionError ?? proxmoxActionMessage}</DiNote>
      ) : null}

      {view === "overview" ? (
        <ProxmoxOverviewTab
          proxmoxDetails={proxmoxDetails}
          proxmoxNode={proxmoxNode}
          proxmoxVMID={proxmoxVMID}
          effectiveKind={effectiveKind}
        />
      ) : null}
      {view === "snapshots" ? (
        <ProxmoxSnapshotsTab
          proxmoxDetails={proxmoxDetails}
          effectiveKind={effectiveKind}
          canRunProxmoxQuickActions={canRunProxmoxQuickActions}
          proxmoxActionRunning={proxmoxActionRunning}
          onRunProxmoxQuickAction={onRunProxmoxQuickAction}
        />
      ) : null}
      {view === "tasks" ? (
        <ProxmoxTasksTab tasks={proxmoxDetails.tasks ?? []} proxmoxCollectorID={proxmoxCollectorID} onRetry={onRetry} />
      ) : null}
      {view === "backup" ? <ProxmoxBackupTab proxmoxDetails={proxmoxDetails} /> : null}
      {view === "storage" ? <ProxmoxStorageTab proxmoxDetails={proxmoxDetails} /> : null}
      {view === "network" ? <ProxmoxNetworkTab networkInterfaces={networkInterfaces} /> : null}
      {view === "firewall" ? <ProxmoxFirewallTab proxmoxDetails={proxmoxDetails} /> : null}
      {view === "ha" ? <ProxmoxHATab proxmoxDetails={proxmoxDetails} /> : null}
      {view === "ceph" ? <ProxmoxCephTab proxmoxDetails={proxmoxDetails} /> : null}
      {view === "console" ? (
        <ProxmoxConsoleTab assetId={nodeId} proxmoxNode={node} proxmoxVMID={proxmoxVMID} effectiveKind={effectiveKind} />
      ) : null}
      {view === "replication" ? <ProxmoxReplicationTab proxmoxNode={node} proxmoxCollectorID={proxmoxCollectorID} /> : null}
      {view === "cluster" ? <ProxmoxClusterTab clusterStatus={clusterStatus} /> : null}
      {view === "metrics" ? <ProxmoxMetricsTab assetId={nodeId} proxmoxCollectorID={proxmoxCollectorID} /> : null}
      {view === "logs" ? <ProxmoxLogsTab proxmoxNode={node} proxmoxCollectorID={proxmoxCollectorID} /> : null}
      {view === "updates" ? <ProxmoxUpdatesTab proxmoxNode={node} proxmoxCollectorID={proxmoxCollectorID} /> : null}
      {view === "certificates" ? <ProxmoxCertificatesTab proxmoxNode={node} proxmoxCollectorID={proxmoxCollectorID} /> : null}
    </DiPage>
  );
}

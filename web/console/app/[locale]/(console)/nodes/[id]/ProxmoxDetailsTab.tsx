"use client";

import { useMemo, useState } from "react";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { SubTabBar } from "../../../../components/ui/SubTabBar";
import type {
  ClusterStatusEntry,
  NetworkInterface,
  ProxmoxDetails,
} from "./nodeDetailTypes";
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
};

const ALL_PROXMOX_TABS = [
  { id: "overview", label: "Overview" },
  { id: "snapshots", label: "Snapshots" },
  { id: "tasks", label: "Tasks" },
  { id: "firewall", label: "Firewall" },
  { id: "backup", label: "Backup" },
  { id: "storage", label: "Storage" },
  { id: "network", label: "Network" },
  { id: "ha", label: "HA" },
  { id: "ceph", label: "Ceph" },
  { id: "console", label: "Console" },
  { id: "replication", label: "Replication" },
  { id: "cluster", label: "Cluster" },
  { id: "metrics", label: "Metrics" },
  { id: "logs", label: "Logs" },
  { id: "updates", label: "Updates" },
  { id: "certificates", label: "Certificates" },
] as const;

type ProxmoxTabId = (typeof ALL_PROXMOX_TABS)[number]["id"];

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
}: ProxmoxDetailsTabProps) {
  const [activeTab, setActiveTab] = useState<ProxmoxTabId>("overview");

  // Derived flags from existing proxmoxDetails (used as fallback when
  // shaping visible tabs).
  const hasCephFromData = !!(
    proxmoxDetails?.ceph_status?.health?.status ||
    (proxmoxDetails?.ceph_osds && proxmoxDetails.ceph_osds.length > 0)
  );
  const hasHAFromData = !!(
    proxmoxDetails?.ha?.match ||
    (proxmoxDetails?.ha?.resources && proxmoxDetails.ha.resources.length > 0)
  );
  const isVMorCT = effectiveKind === "qemu" || effectiveKind === "lxc";
  const isNode = effectiveKind === "node";

  const visibleTabs = useMemo(() => {
    const hasCeph = hasCephFromData;
    const hasHA = hasHAFromData;
    return ALL_PROXMOX_TABS.filter((tab) => {
      if (tab.id === "ceph" && !hasCeph) return false;
      if (tab.id === "console" && !isVMorCT) return false;
      if (tab.id === "ha" && !hasHA) return false;
      if (tab.id === "cluster" && !isNode) return false;
      return true;
    });
  }, [hasCephFromData, hasHAFromData, isVMorCT, isNode]);

  // If the active tab got hidden, fall back to overview.
  const effectiveTab: ProxmoxTabId =
    visibleTabs.some((t) => t.id === activeTab) ? activeTab : "overview";

  if (proxmoxLoading && !proxmoxDetails) {
    return (
      <Card className="mb-4">
        <p className="text-sm text-[var(--muted)]">Loading Proxmox details...</p>
      </Card>
    );
  }

  if (proxmoxError && !proxmoxDetails) {
    return (
      <Card className="mb-4">
        <div className="flex flex-col items-center justify-center gap-3 py-8">
          <p className="text-xs text-[var(--bad)]">{proxmoxError}</p>
          <Button size="sm" onClick={onRetry}>Retry</Button>
        </div>
      </Card>
    );
  }

  if (!proxmoxDetails) {
    return (
      <Card className="mb-4">
        <div className="flex flex-col items-center justify-center gap-2 py-12">
          <p className="text-sm font-medium text-[var(--text)]">No Proxmox details yet</p>
          <p className="max-w-sm text-center text-xs text-[var(--muted)]">
            Select this tab again after the asset is discovered by the Proxmox collector.
          </p>
        </div>
      </Card>
    );
  }

  return (
    <div className="space-y-0">
      <SubTabBar
        tabs={visibleTabs as unknown as Array<{ id: string; label: string }>}
        activeTab={effectiveTab}
        onTabChange={(id) => { setActiveTab(id as ProxmoxTabId); }}
      />

      {effectiveTab === "overview" && (
        <ProxmoxOverviewTab
          proxmoxDetails={proxmoxDetails}
          proxmoxNode={proxmoxNode}
          proxmoxVMID={proxmoxVMID}
          effectiveKind={effectiveKind}
        />
      )}

      {effectiveTab === "snapshots" && (
        <ProxmoxSnapshotsTab
          proxmoxDetails={proxmoxDetails}
          effectiveKind={effectiveKind}
          canRunProxmoxQuickActions={canRunProxmoxQuickActions}
          proxmoxActionRunning={proxmoxActionRunning}
          onRunProxmoxQuickAction={onRunProxmoxQuickAction}
        />
      )}

      {effectiveTab === "tasks" && (
        <ProxmoxTasksTab
          tasks={proxmoxDetails.tasks ?? []}
          proxmoxCollectorID={proxmoxCollectorID}
          onRetry={onRetry}
        />
      )}

      {effectiveTab === "firewall" && (
        <ProxmoxFirewallTab proxmoxDetails={proxmoxDetails} />
      )}

      {effectiveTab === "backup" && (
        <ProxmoxBackupTab proxmoxDetails={proxmoxDetails} />
      )}

      {effectiveTab === "storage" && (
        <ProxmoxStorageTab
          proxmoxDetails={proxmoxDetails}
          clusterStatus={clusterStatus}
          networkInterfaces={networkInterfaces}
        />
      )}

      {effectiveTab === "network" && (
        <ProxmoxNetworkTab networkInterfaces={networkInterfaces} />
      )}

      {effectiveTab === "ha" && hasHAFromData && (
        <ProxmoxHATab proxmoxDetails={proxmoxDetails} />
      )}

      {effectiveTab === "ceph" && hasCephFromData && (
        <ProxmoxCephTab proxmoxDetails={proxmoxDetails} />
      )}

      {effectiveTab === "console" && isVMorCT && (
        <ProxmoxConsoleTab
          proxmoxNode={proxmoxNode}
          proxmoxVMID={proxmoxVMID}
          effectiveKind={effectiveKind}
          proxmoxCollectorID={proxmoxCollectorID}
        />
      )}

      {effectiveTab === "replication" && (
        <ProxmoxReplicationTab
          proxmoxNode={proxmoxNode}
          proxmoxCollectorID={proxmoxCollectorID}
        />
      )}

      {effectiveTab === "cluster" && isNode && (
        <ProxmoxClusterTab clusterStatus={clusterStatus} />
      )}

      {effectiveTab === "metrics" && (
        <ProxmoxMetricsTab
          assetId={nodeId}
          proxmoxCollectorID={proxmoxCollectorID}
        />
      )}

      {effectiveTab === "logs" && (
        <ProxmoxLogsTab
          proxmoxNode={proxmoxNode}
          proxmoxCollectorID={proxmoxCollectorID}
        />
      )}

      {effectiveTab === "updates" && (
        <ProxmoxUpdatesTab
          proxmoxNode={proxmoxNode}
          proxmoxCollectorID={proxmoxCollectorID}
        />
      )}

      {effectiveTab === "certificates" && (
        <ProxmoxCertificatesTab
          proxmoxNode={proxmoxNode}
          proxmoxCollectorID={proxmoxCollectorID}
        />
      )}
    </div>
  );
}

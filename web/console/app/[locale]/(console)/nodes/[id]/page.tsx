"use client";

import { useEffect, useState } from "react";
import { ArrowLeft } from "lucide-react";
import { useParams } from "next/navigation";
import { useRouter, Link } from "../../../../../i18n/navigation";
import { Card } from "../../../../components/ui/Card";
import { DeviceIdentityBar } from "../../../../components/DeviceIdentityBar";
import type { Asset } from "../../../../console/models";
import { DeviceCapabilityGrid } from "./DeviceCapabilityGrid";
import { useFastStatus, useGroupLabelByID, useSlowStatus, useStatusControls } from "../../../../contexts/StatusContext";
import { useConnectedAgents } from "../../../../hooks/useConnectedAgents";
import { ClusterTopologySection } from "./ClusterTopologySection";
import { DockerStackOverviewTab } from "./DockerStackOverviewTab";
import { NodeDeleteConfirmModal } from "./NodeDeleteConfirmModal";
import { NodeEditModal } from "./NodeEditModal";
import { renderNodeDetailPanel } from "./nodePanelRenderers";
import { useNodeDeleteFlow } from "./useNodeDeleteFlow";
import { useNodeDetailData } from "./useNodeDetailData";
import { useNodeDetailPageModel } from "./useNodeDetailPageModel";
import { useNodePanelRenderContext } from "./useNodePanelRenderContext";
import { useNodeDetailRouting } from "./useNodeDetailRouting";
import { HierarchyBreadcrumb } from "./HierarchyBreadcrumb";
import { GroupPathBreadcrumb } from "./GroupPathBreadcrumb";
import { FacetTabs } from "./FacetTabs";
import { ContainsSummary } from "./ContainsSummary";

export default function NodeDetailPage() {
  const params = useParams();
  const router = useRouter();
  const nodeId = params.id as string;
  const status = useFastStatus();
  const slowStatus = useSlowStatus();
  const { fetchStatus } = useStatusControls();
  const groupLabelByID = useGroupLabelByID();

  const { connectedAgentIds } = useConnectedAgents();
  const nodeHasAgent = connectedAgentIds.has(nodeId);

  const {
    showDeleteConfirm,
    deleteConfirmInput,
    deleting,
    deleteError,
    openDeleteConfirm,
    setDeleteConfirmInput,
    cancelDeleteConfirm,
    confirmDelete,
  } = useNodeDeleteFlow({ nodeId });

  const [showEditModal, setShowEditModal] = useState(false);

  const {
    asset,
    telemetryOverview,
    freshnessClass,
    groupName,
    metadata,
    proxmoxNode,
    proxmoxVMID,
    isProxmoxAsset,
    isDockerContainer,
    isDockerStack,
    dockerContainerId,
    dockerStackName,
    dockerStackAssetId,
    dockerStackHostId,
    canonicalDockerHostNodeID,
    fallbackDockerHostNodeID,
    effectiveKind,
    proxmoxTarget,
    isInfra,
    isLinuxAgentNode,
    supportsServiceListing,
    supportsPackageListing,
    supportsScheduleListing,
    supportsNetworkListing,
    supportsNetworkActions,
    supportsLogQuery,
    logQueryModeLabel,
    nodeNetworkMethodOptions,
    nodeNetworkControlsLabel,
    nodeNetworkControlsHint,
    infraCategories,
    panelContext,
    panels,
    dockerHostForPanel,
  } = useNodeDetailPageModel({
    nodeId,
    assets: status?.assets,
    telemetry: status?.telemetryOverview,
    groupLabelByID,
    nodeHasAgent,
  });

  const { activePanel, activeSub, activeDetail, activeTabForHook } = useNodeDetailRouting({
    nodeId,
    canonicalDockerHostNodeID,
    fallbackDockerHostNodeID,
  });

  const openPanel = (panel: string) => {
    router.push(`/nodes/${encodeURIComponent(nodeId)}?panel=${panel}`);
  };

  const openSystemDetail = (detail: "cpu" | "memory" | "storage" | "network") => {
    router.push(`/nodes/${encodeURIComponent(nodeId)}?panel=system&detail=${detail}`);
  };

  const closeSystemDetail = () => {
    router.replace(`/nodes/${encodeURIComponent(nodeId)}?panel=system`);
  };

  const {
    telemetryDetails,
    telemetryLoading,
    telemetryWindow,
    setTelemetryWindow,
    logEvents,
    logsLoading,
    logsError,
    logLevelFilter,
    setLogLevelFilter,
    logMode,
    setLogMode,
    logWindow,
    setLogWindow,
    journalSince,
    setJournalSince,
    journalUntil,
    setJournalUntil,
    journalUnit,
    setJournalUnit,
    journalPriority,
    setJournalPriority,
    journalQuery,
    setJournalQuery,
    journalLiveTail,
    setJournalLiveTail,
    refreshLogs,
    actionRuns,
    actionsLoading,
    expandedActionId,
    setExpandedActionId,
    proxmoxDetails,
    proxmoxLoading,
    proxmoxError,
    clusterStatus,
    networkInterfaces,
    proxmoxCollectorID,
    proxmoxActionRunning,
    proxmoxActionMessage,
    proxmoxActionError,
    nodeActionRunning,
    nodeActionMessage,
    nodeActionError,
    nodeNetworkActionRunning,
    nodeNetworkActionMessage,
    nodeNetworkActionError,
    fetchProxmoxDetails,
    runProxmoxQuickAction,
    runNodeQuickAction,
    runNodeNetworkAction,
  } = useNodeDetailData({
    activeTab: activeTabForHook,
    nodeId,
    isProxmoxAsset,
    isInfra,
    effectiveKind,
    proxmoxNode,
    proxmoxTarget,
    proxmoxCollectorHint: asset?.metadata?.collector_id?.trim() ?? "",
  });

  useEffect(() => {
    if (!supportsLogQuery && logMode === "journal") {
      setLogMode("stored");
    }
  }, [logMode, setLogMode, supportsLogQuery]);

  const canRunProxmoxQuickActions = isProxmoxAsset && (effectiveKind === "qemu" || effectiveKind === "lxc");
  const renderAsset: Asset = asset ?? {
    id: nodeId,
    name: nodeId,
    type: "unknown",
    source: "unknown",
    status: "unknown",
    last_seen_at: "",
    metadata: {},
  };
  const panelRenderContext = useNodePanelRenderContext({
    activePanel,
    activeSub,
    activeDetail,
    nodeId,
    asset: renderAsset,
    metadata,
    telemetryOverview,
    telemetryDetails,
    telemetryLoading: telemetryLoading ?? false,
    telemetryWindow,
    infraCategories,
    isDockerContainer,
    isDockerStack,
    isProxmoxAsset,
    effectiveKind,
    dockerHostForPanel,
    dockerContainerId,
    dockerStackHostId,
    dockerStackName,
    dockerStackAssetId,
    openPanel,
    openSystemDetail,
    closeSystemDetail,
    replaceDockerSub: (sub) => router.replace(`/nodes/${encodeURIComponent(nodeId)}?panel=docker&sub=${sub}`),
    telemetryWindowChange: setTelemetryWindow,
    logsLoading,
    logsError,
    logEvents,
    logLevelFilter,
    setLogLevelFilter,
    logMode,
    setLogMode,
    supportsLogQuery,
    logQueryModeLabel,
    logWindow,
    setLogWindow,
    journalSince,
    setJournalSince,
    journalUntil,
    setJournalUntil,
    journalUnit,
    setJournalUnit,
    journalPriority,
    setJournalPriority,
    journalQuery,
    setJournalQuery,
    journalLiveTail,
    setJournalLiveTail,
    refreshLogs,
    actionsLoading,
    actionRuns,
    expandedActionId,
    setExpandedActionId,
    proxmoxDetails,
    proxmoxLoading,
    proxmoxError,
    proxmoxNode,
    proxmoxVMID,
    canRunProxmoxQuickActions,
    proxmoxActionRunning,
    clusterStatus,
    networkInterfaces,
    proxmoxCollectorID,
    fetchProxmoxDetails,
    runProxmoxQuickAction,
    proxmoxCollectorId: proxmoxCollectorID || null,
    proxmoxActionMessage,
    proxmoxActionError,
  });

  if (!asset) {
    return (
      <Card>
        <div className="flex flex-col items-center justify-center gap-2 py-12">
          <p className="text-sm font-medium text-[var(--text)]">Device not found</p>
          <p className="max-w-sm text-center text-xs text-[var(--muted)]">Couldn&apos;t find a device with that ID. It may not have checked in yet.</p>
        </div>
      </Card>
    );
  }

  return (
    <>
      <Link
        href="/nodes"
        className="group inline-flex items-center gap-1.5 text-xs text-[var(--muted)] hover:text-[var(--text)] transition-colors mb-3"
        style={{ transitionDuration: "var(--dur-fast)" }}
      >
        <ArrowLeft size={14} className="transition-transform group-hover:-translate-x-0.5" style={{ transitionDuration: "var(--dur-fast)" }} />
        Devices
      </Link>
      <GroupPathBreadcrumb groupId={asset.group_id} groups={slowStatus?.groups ?? []} />
      <HierarchyBreadcrumb assetID={nodeId} />
      <DeviceIdentityBar
        asset={asset}
        telemetry={telemetryOverview}
        groupName={groupName}
        freshnessStatus={freshnessClass === "ok" ? "ok" : freshnessClass === "pending" ? "pending" : "bad"}
        agentConnected={nodeHasAgent}
        activePanel={activePanel}
        onBack={() => router.push(`/nodes/${encodeURIComponent(nodeId)}`)}
        onQuickAction={nodeHasAgent ? (command) => void runNodeQuickAction(command, command) : undefined}
        onEdit={() => setShowEditModal(true)}
        onDelete={openDeleteConfirm}
        onAddConnection={() => router.push(`/nodes/${encodeURIComponent(nodeId)}?panel=connect&adding=true`)}
      />
      <FacetTabs />

      <div
        key={activePanel ?? "dashboard"}
        className="animate-[slide-in_150ms_ease-out]"
      >
        {/* Dashboard view — no active panel */}
        {!activePanel && (
          <>
            <ContainsSummary assetID={nodeId} />
            {isDockerStack ? (
              <DockerStackOverviewTab
                hostId={dockerStackHostId}
                stackName={dockerStackName}
                stackAssetId={dockerStackAssetId}
              />
            ) : (
              <DeviceCapabilityGrid
                panels={panels}
                context={panelContext}
                onOpenPanel={openPanel}
              />
            )}
            {isProxmoxAsset && effectiveKind === "node" && clusterStatus.length > 0 ? (
              <ClusterTopologySection
                clusterStatus={clusterStatus}
                haResources={proxmoxDetails?.ha?.resources}
                assets={status?.assets ?? []}
              />
            ) : null}
          </>
        )}

        {/* Panel content */}
        {renderNodeDetailPanel(panelRenderContext)}
      </div>

      <NodeDeleteConfirmModal
        open={showDeleteConfirm}
        assetName={asset.name}
        confirmInput={deleteConfirmInput}
        deleting={deleting}
        error={deleteError}
        onConfirmInputChange={setDeleteConfirmInput}
        onCancel={cancelDeleteConfirm}
        onConfirm={() => { void confirmDelete(); }}
      />
      {showEditModal && asset && (
        <NodeEditModal
          asset={asset}
          groups={slowStatus?.groups ?? []}
          onClose={() => setShowEditModal(false)}
          onSave={() => fetchStatus()}
        />
      )}
    </>
  );
}

"use client";

import { useEffect, useMemo } from "react";
import { useParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { useRouter, Link } from "../../../../../i18n/navigation";
import { DeviceIdentityBar } from "../../../../components/DeviceIdentityBar";
import type { Asset } from "../../../../console/models";
import { useFastStatus, useGroupLabelByID, useSlowStatus, useStatusControls } from "../../../../contexts/StatusContext";
import { useConnectedAgents } from "../../../../hooks/useConnectedAgents";
import { ClusterTopologySection } from "./ClusterTopologySection";
import { DockerStackOverviewTab } from "./DockerStackOverviewTab";
import { renderNodeDetailPanel } from "./nodePanelRenderers";
import { useNodeDetailData } from "./useNodeDetailData";
import { useNodeDetailPageModel } from "./useNodeDetailPageModel";
import { useNodePanelRenderContext } from "./useNodePanelRenderContext";
import { useNodeDetailRouting } from "./useNodeDetailRouting";
import { HierarchyBreadcrumb } from "./HierarchyBreadcrumb";
import { DeviceHeaderActions } from "./DeviceHeaderActions";
import { DeviceContainerActions } from "./DeviceContainerActions";
import { DeviceTabBar } from "./DeviceTabBar";
import { DeviceDetails } from "./DeviceDetails";
import { DeviceEditCard } from "./DeviceEditCard";
import { DeviceUsage } from "./DeviceUsage";
import { DeviceActivity, DeviceServicesList } from "./DeviceOverviewLists";
import { DeviceLogsTab, DeviceOverviewLayout, DeviceSettingsTab, DeviceSystemTab } from "./DeviceTabShells";
import { buildDeviceTabs, logsViewsOf, systemViewsOf, tabOfPanel } from "./deviceTabsModel";
import { useDeviceActivity } from "./useDeviceActivity";
import { guestsOf } from "./deviceStatusModel";
import { useAuth } from "../../../../contexts/AuthContext";
import { canAccessHostData, hasWriteRole, isNodePanelAllowed } from "../../../../lib/roles";
import { OperatorAccessRequired } from "../../../../components/OperatorAccessRequired";

export default function NodeDetailPage() {
  const params = useParams();
  const router = useRouter();
  const nodeId = params.id as string;
  const status = useFastStatus();
  const slowStatus = useSlowStatus();
  const { fetchStatus } = useStatusControls();
  const groupLabelByID = useGroupLabelByID();
  const { user } = useAuth();
  const t = useTranslations("devices.page");
  const tAccess = useTranslations("common.operatorAccess");
  const canWrite = hasWriteRole(user?.role);
  // Host journal queries are host data; viewers keep stored log history only.
  const hostDataAllowed = canAccessHostData(user?.role);

  const { connectedAgentIds } = useConnectedAgents();
  const nodeHasAgent = connectedAgentIds.has(nodeId);

  const model = useNodeDetailPageModel({
    nodeId,
    assets: status?.assets,
    telemetry: status?.telemetryOverview,
    groupLabelByID,
    nodeHasAgent,
  });
  const { asset, metadata, isProxmoxAsset, isDockerContainer, isDockerStack, effectiveKind, infraCategories, panels } = model;

  const { activePanel, activeSub, activeDetail, activeTabForHook } = useNodeDetailRouting({
    nodeId,
    canonicalDockerHostNodeID: model.canonicalDockerHostNodeID,
    fallbackDockerHostNodeID: model.fallbackDockerHostNodeID,
  });

  const visiblePanels = panels.filter((panel) => isNodePanelAllowed(panel.id, user?.role, { isDockerContainer }));
  const panelIds = visiblePanels.map((panel) => panel.id);
  const categorySlugs = useMemo(() => new Set<string>(infraCategories.keys()), [infraCategories]);
  const currentTab = tabOfPanel(activePanel, categorySlugs);
  const tabs = buildDeviceTabs({
    panelIds,
    categorySlugs,
    isDockerContainer,
    isDockerStack,
    isProxmoxHost: isProxmoxAsset && effectiveKind === "node",
    activePanel,
  });
  const activePanelBlocked = activePanel !== null && !isNodePanelAllowed(activePanel, user?.role, { isDockerContainer });

  const openPanel = (panel: string) => router.push(`/nodes/${encodeURIComponent(nodeId)}?panel=${panel}`);

  const data = useNodeDetailData({
    // Settings shows the agent, the collector and connections together; the collector loads as "settings".
    activeTab: currentTab === "settings" ? "settings" : activeTabForHook,
    nodeId,
    isProxmoxAsset,
    isInfra: model.isInfra,
    effectiveKind,
    proxmoxNode: model.proxmoxNode,
    proxmoxTarget: model.proxmoxTarget,
    proxmoxCollectorHint: asset?.metadata?.collector_id?.trim() ?? "",
  });
  const activity = useDeviceActivity(nodeId, currentTab === "overview" && !!asset && !isDockerStack);

  const journalAllowed = model.supportsLogQuery && hostDataAllowed;
  const { logMode, setLogMode } = data;
  useEffect(() => {
    if (!journalAllowed && logMode === "journal") setLogMode("stored");
  }, [journalAllowed, logMode, setLogMode]);

  const renderAsset: Asset = asset ?? {
    id: nodeId, name: nodeId, type: "unknown", source: "unknown", status: "unknown", last_seen_at: "", metadata: {},
  };
  const panelRenderContext = useNodePanelRenderContext({
    ...data,
    activePanel,
    activeSub,
    activeDetail,
    nodeId,
    asset: renderAsset,
    metadata,
    telemetryOverview: model.telemetryOverview,
    telemetryLoading: data.telemetryLoading ?? false,
    infraCategories,
    isDockerContainer,
    isDockerStack,
    isProxmoxAsset,
    effectiveKind,
    dockerHostForPanel: model.dockerHostForPanel,
    dockerContainerId: model.dockerContainerId,
    dockerStackHostId: model.dockerStackHostId,
    dockerStackName: model.dockerStackName,
    dockerStackAssetId: model.dockerStackAssetId,
    openPanel,
    openSystemDetail: (detail) => router.push(`/nodes/${encodeURIComponent(nodeId)}?panel=system&detail=${detail}`),
    closeSystemDetail: () => router.replace(`/nodes/${encodeURIComponent(nodeId)}?panel=system`),
    replaceDockerSub: (sub) => router.replace(`/nodes/${encodeURIComponent(nodeId)}?panel=docker&sub=${sub}`),
    telemetryWindowChange: data.setTelemetryWindow,
    supportsLogQuery: journalAllowed,
    logQueryModeLabel: model.logQueryModeLabel,
    proxmoxNode: model.proxmoxNode,
    proxmoxVMID: model.proxmoxVMID,
    canRunProxmoxQuickActions: isProxmoxAsset && (effectiveKind === "qemu" || effectiveKind === "lxc"),
    proxmoxCollectorId: data.proxmoxCollectorID || null,
  });

  if (!asset) {
    if (!status) return null;
    return (
      <div className="dv">
        <nav className="dv-crumb" aria-label={t("crumbLabel")}><Link href="/nodes">{t("crumb")}</Link></nav>
        <div className="dv-sheet dv-empty dv-gone">
          <b>{t("notFound", { id: nodeId })}</b>
          <Link href="/nodes" className="dv-btn dv-empty-act">{t("crumb")}</Link>
        </div>
      </div>
    );
  }

  const panelAt = (panel: string) => renderNodeDetailPanel({ ...panelRenderContext, activePanel: panel });
  const has = (panel: string) => panelIds.includes(panel);
  const offers = { terminal: has("terminal"), files: has("files"), desktop: has("desktop") };

  let body;
  if (activePanelBlocked) {
    body = <OperatorAccessRequired title={tAccess("title")} feature="hostData" />;
  } else if (currentTab === "overview") {
    body = isDockerStack ? (
      <DockerStackOverviewTab hostId={model.dockerStackHostId} stackName={model.dockerStackName} stackAssetId={model.dockerStackAssetId} />
    ) : (
      <DeviceOverviewLayout
        main={
          <>
            <DeviceUsage
              asset={asset}
              telemetry={model.telemetryOverview}
              details={data.telemetryDetails}
              loading={data.telemetryLoading ?? false}
              window={data.telemetryWindow}
              onWindowChange={data.setTelemetryWindow}
            />
            <DeviceServicesList services={model.webServicesForNode} />
            <DeviceActivity nodeId={nodeId} events={activity.events} loaded={activity.loaded} />
            {isProxmoxAsset && effectiveKind === "node" && data.clusterStatus.length > 0 ? (
              <ClusterTopologySection clusterStatus={data.clusterStatus} haResources={data.proxmoxDetails?.ha?.resources} assets={status?.assets ?? []} />
            ) : null}
          </>
        }
        details={
          <DeviceDetails
            asset={asset}
            nodeHasAgent={nodeHasAgent}
            host={model.parentInfraHost}
            groups={slowStatus?.groups ?? []}
            guests={isProxmoxAsset && effectiveKind === "node" ? guestsOf(asset, status?.assets ?? []) : null}
          />
        }
      />
    );
  } else if (currentTab === "system") {
    body = (
      <DeviceSystemTab asset={asset} views={systemViewsOf(panelIds, categorySlugs)} current={activePanel ?? "system"}>
        {renderNodeDetailPanel(panelRenderContext)}
      </DeviceSystemTab>
    );
  } else if (currentTab === "logs") {
    body = (
      <DeviceLogsTab nodeId={nodeId} views={logsViewsOf(panelIds)} current={activePanel ?? "logs"}>
        {renderNodeDetailPanel(panelRenderContext)}
      </DeviceLogsTab>
    );
  } else if (currentTab === "settings") {
    body = (
      <DeviceSettingsTab asset={asset}>
        {asset.source === "manual" && canWrite ? (
          <DeviceEditCard asset={asset} groups={slowStatus?.groups ?? []} onSaved={() => void fetchStatus()} />
        ) : null}
        {has("agent-settings") ? panelAt("agent-settings") : null}
        {isProxmoxAsset && model.isInfra ? panelAt("settings") : null}
        {panelAt("connect")}
      </DeviceSettingsTab>
    );
  } else {
    body = <div className="dv-tab">{renderNodeDetailPanel(panelRenderContext)}</div>;
  }

  return (
    <div className="dv">
      <HierarchyBreadcrumb assetID={nodeId} name={asset.name} />
      <DeviceIdentityBar
        asset={asset}
        actions={isDockerContainer ? <DeviceContainerActions asset={asset} /> : (
          <DeviceHeaderActions
            asset={asset}
            offers={offers}
            onPower={nodeHasAgent && canWrite ? (command) => void data.runNodeQuickAction(command, command) : undefined}
          />
        )}
      />
      {data.nodeActionError || data.nodeActionMessage ? (
        <p className={`dv-note${data.nodeActionError ? " bad" : ""}`} role="status">{data.nodeActionError ?? data.nodeActionMessage}</p>
      ) : null}
      <DeviceTabBar nodeId={nodeId} tabs={tabs} current={currentTab} />
      <div key={currentTab}>{body}</div>
    </div>
  );
}

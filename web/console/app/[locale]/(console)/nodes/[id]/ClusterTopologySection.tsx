"use client";

import { useMemo, useState } from "react";
import dynamic from "next/dynamic";
import { useTranslations } from "next-intl";
import { buildClusterFlowModel } from "./clusterTopologyFlowModel";
import type { ClusterFlowWords } from "./clusterTopologyPresentation";
import { DiEmpty, DiPage, DiSection, DiSheet } from "./proxmox/DiList";
import { ClusterTopologyHeader } from "./ClusterTopologyHeader";
import { ClusterTopologyListView } from "./ClusterTopologyListView";
import { useClusterTopologyDerivedData } from "./useClusterTopologyDerivedData";
import { useClusterTopologyGraphController } from "./useClusterTopologyGraphController";
import { useClusterTopologyGuestLinking } from "./useClusterTopologyGuestLinking";
import type {
  ClusterTopologySectionProps,
  TopologyView,
} from "./clusterTopologyTypes";

const ClusterTopologyGraphView = dynamic(
  () => import("./ClusterTopologyGraphView").then((mod) => ({ default: mod.ClusterTopologyGraphView })),
  {
    ssr: false,
    loading: () => <GraphLoading />,
  },
);

function GraphLoading() {
  const t = useTranslations("devices-proxmox.topology");
  return (
    <DiSheet>
      <DiEmpty title={t("loadingGraph")} />
    </DiSheet>
  );
}

/** The Proxmox cluster on a node's Overview: nodes, their guests and the hosts guests are linked to, as a graph or a list. */
export function ClusterTopologySection({ clusterStatus, haResources, assets = [] }: ClusterTopologySectionProps) {
  const t = useTranslations("devices-proxmox.topology");
  const [topologyView, setTopologyView] = useState<TopologyView>("graph");
  const words = useMemo<ClusterFlowWords>(() => ({
    local: t("local"),
    guests: (count) => t("guests", { count }),
    linkedTo: (name) => t("linkedTo", { name }),
    links: (count) => t("links", { count }),
    auto: t("auto"),
    manual: t("manual"),
    missing: t("missing"),
    missingTarget: t("missingTarget"),
    autoLink: (guest, host) => t("autoLink", { guest, host }),
    manualLink: (guest, host) => t("manualLink", { guest, host }),
    missingLink: (target) => t("missingLink", { target }),
  }), [t]);
  const {
    clusterEntry,
    nodeEntries,
    haByNode,
    guestsByNode,
    apiHosts,
    apiHostsByID,
    apiHostsByName,
    hostIdentityByID,
    guestAssets,
    guestsByID,
    proxmoxNodeAssetIDByName,
  } = useClusterTopologyDerivedData({
    clusterStatus,
    haResources,
    assets,
  });

  const {
    guestRunsOn,
    linkDrafts,
    loadingGuestLinks,
    savingGuestID,
    linkErrors,
    autoLinkingGuestID,
    suggestedHostForGuest,
    setGuestLinkDraft,
    saveGuestLink,
    clearGuestLink,
  } = useClusterTopologyGuestLinking({
    guestAssets,
    apiHosts,
    apiHostsByID,
    apiHostsByName,
    hostIdentityByID,
  });

  const {
    graphSelectedGuestID,
    openAssetDetails,
    onGraphNodeClick,
    onGraphNodeDoubleClick,
    onGraphEdgeClick,
    onGraphPaneClick,
  } = useClusterTopologyGraphController({
    guestsByID,
  });

  const selectedGraphGuest = graphSelectedGuestID ? guestsByID.get(graphSelectedGuestID) : undefined;
  const selectedGraphGuestMapping = selectedGraphGuest ? (guestRunsOn[selectedGraphGuest.id] ?? null) : null;
  const selectedGraphLinkedHost = selectedGraphGuestMapping
    ? apiHostsByID.get(selectedGraphGuestMapping.target_asset_id)
    : undefined;
  const selectedGraphDraftTargetID = selectedGraphGuest ? (linkDrafts[selectedGraphGuest.id] ?? "") : "";
  const selectedGraphSaving = selectedGraphGuest
    ? (savingGuestID === selectedGraphGuest.id || autoLinkingGuestID === selectedGraphGuest.id)
    : false;
  const selectedGraphCanSaveMapping = selectedGraphGuest
    ? (selectedGraphDraftTargetID !== ""
      && (!selectedGraphGuestMapping || selectedGraphGuestMapping.target_asset_id !== selectedGraphDraftTargetID))
    : false;
  const selectedGraphSuggestion = selectedGraphGuest ? suggestedHostForGuest(selectedGraphGuest) : undefined;
  const selectedGraphShowNameSuggestion = Boolean(
    selectedGraphGuest
      && !selectedGraphGuestMapping
      && selectedGraphSuggestion
      && selectedGraphDraftTargetID === selectedGraphSuggestion.id,
  );

  const flowModel = useMemo(
    () => buildClusterFlowModel({
      apiHosts,
      apiHostsByID,
      guestAssets,
      guestRunsOn,
      guestsByNode,
      graphSelectedGuestID,
      nodeEntries,
      proxmoxNodeAssetIDByName,
      words,
    }),
    [apiHosts, apiHostsByID, guestAssets, guestRunsOn, guestsByNode, graphSelectedGuestID, nodeEntries, proxmoxNodeAssetIDByName, words],
  );

  if (nodeEntries.length === 0) return null;

  return (
    <DiPage className="di-ct">
      <DiSection
        id="cluster-topology"
        title={clusterEntry?.name ? t("titleNamed", { name: clusterEntry.name }) : t("title")}
        right={
          <ClusterTopologyHeader
            topologyView={topologyView}
            onTopologyViewChange={setTopologyView}
            clusterEntry={clusterEntry}
          />
        }
      >

        {topologyView === "graph" ? (
          <ClusterTopologyGraphView
            flowModel={flowModel}
            onGraphNodeClick={onGraphNodeClick}
            onGraphNodeDoubleClick={onGraphNodeDoubleClick}
            onGraphEdgeClick={onGraphEdgeClick}
            onGraphPaneClick={onGraphPaneClick}
            selectedGraphGuest={selectedGraphGuest}
            selectedGraphGuestMapping={selectedGraphGuestMapping}
            selectedGraphLinkedHost={selectedGraphLinkedHost}
            selectedGraphDraftTargetID={selectedGraphDraftTargetID}
            selectedGraphSaving={selectedGraphSaving}
            selectedGraphCanSaveMapping={selectedGraphCanSaveMapping}
            selectedGraphShowNameSuggestion={selectedGraphShowNameSuggestion}
            selectedGraphError={selectedGraphGuest ? linkErrors[selectedGraphGuest.id] : undefined}
            apiHosts={apiHosts}
            loadingGuestLinks={loadingGuestLinks}
            autoLinkingGuestID={autoLinkingGuestID}
            onSelectedGraphDraftChange={(targetID) => {
              if (!selectedGraphGuest) {
                return;
              }
              setGuestLinkDraft(selectedGraphGuest.id, targetID);
            }}
            onSaveSelectedGuest={() => {
              if (!selectedGraphGuest) {
                return;
              }
              void saveGuestLink(selectedGraphGuest);
            }}
            onClearSelectedGuest={() => {
              if (!selectedGraphGuest) {
                return;
              }
              void clearGuestLink(selectedGraphGuest);
            }}
            onOpenGuest={() => {
              if (!selectedGraphGuest) {
                return;
              }
              openAssetDetails(selectedGraphGuest.id);
            }}
            onOpenLinkedHost={() => {
              if (!selectedGraphLinkedHost) {
                return;
              }
              openAssetDetails(selectedGraphLinkedHost.id);
            }}
          />
        ) : (
          <ClusterTopologyListView
            nodeEntries={nodeEntries}
            haByNode={haByNode}
            guestsByNode={guestsByNode}
            guestRunsOn={guestRunsOn}
            linkDrafts={linkDrafts}
            linkErrors={linkErrors}
            savingGuestID={savingGuestID}
            autoLinkingGuestID={autoLinkingGuestID}
            loadingGuestLinks={loadingGuestLinks}
            apiHosts={apiHosts}
            apiHostsByID={apiHostsByID}
            suggestedHostForGuest={suggestedHostForGuest}
            onDraftChange={(guestID, targetID) => {
              setGuestLinkDraft(guestID, targetID);
            }}
            onSaveGuestLink={(guest) => { void saveGuestLink(guest); }}
            onClearGuestLink={(guest) => { void clearGuestLink(guest); }}
          />
        )}
      </DiSection>
    </DiPage>
  );
}

"use client";

import { useMemo, useState } from "react";
import { Card } from "../../../../components/ui/Card";
import { buildClusterFlowModel } from "./clusterTopologyFlowModel";
import { ClusterTopologyHeader } from "./ClusterTopologyHeader";
import { ClusterTopologyGraphView } from "./ClusterTopologyGraphView";
import { ClusterTopologyListView } from "./ClusterTopologyListView";
import { useClusterTopologyDerivedData } from "./useClusterTopologyDerivedData";
import { useClusterTopologyGraphController } from "./useClusterTopologyGraphController";
import { useClusterTopologyGuestLinking } from "./useClusterTopologyGuestLinking";
import type {
  ClusterTopologySectionProps,
  TopologyView,
} from "./clusterTopologyTypes";

export function ClusterTopologySection({ clusterStatus, haResources, assets = [] }: ClusterTopologySectionProps) {
  const [topologyView, setTopologyView] = useState<TopologyView>("graph");
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
    }),
    [apiHosts, apiHostsByID, guestAssets, guestRunsOn, guestsByNode, graphSelectedGuestID, nodeEntries, proxmoxNodeAssetIDByName],
  );

  if (nodeEntries.length === 0) return null;

  return (
    <Card className="mb-4">
      <div className="space-y-4">
        <ClusterTopologyHeader
          topologyView={topologyView}
          onTopologyViewChange={setTopologyView}
          clusterEntry={clusterEntry}
        />

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
      </div>
    </Card>
  );
}

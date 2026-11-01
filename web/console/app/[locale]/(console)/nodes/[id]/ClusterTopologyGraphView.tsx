"use client";

import { type MouseEvent } from "react";
import {
  Background,
  Controls,
  ReactFlow,
  type Edge,
  type Node,
} from "@xyflow/react";
import { useTranslations } from "next-intl";
import type { Asset } from "../../../../console/models";
import { friendlyTypeLabel } from "../../../../console/taxonomy";
import { ClusterGuestLinkEditor } from "./ClusterTopologyListView";
import type { ClusterTopologyNodeData } from "./clusterTopologyFlowModel";
import type { AssetDependency } from "./clusterTopologyTypes";

type ClusterTopologyGraphViewProps = {
  flowModel: {
    nodes: Node<ClusterTopologyNodeData>[];
    edges: Edge[];
  };
  onGraphNodeClick: (_event: MouseEvent, node: Node<ClusterTopologyNodeData>) => void;
  onGraphNodeDoubleClick: (_event: MouseEvent, node: Node<ClusterTopologyNodeData>) => void;
  onGraphEdgeClick: (_event: MouseEvent, edge: Edge) => void;
  onGraphPaneClick: () => void;
  selectedGraphGuest?: Asset;
  selectedGraphGuestMapping: AssetDependency | null;
  selectedGraphLinkedHost?: Asset;
  selectedGraphDraftTargetID: string;
  selectedGraphSaving: boolean;
  selectedGraphCanSaveMapping: boolean;
  selectedGraphShowNameSuggestion: boolean;
  selectedGraphError?: string;
  apiHosts: Asset[];
  loadingGuestLinks: boolean;
  autoLinkingGuestID: string | null;
  onSelectedGraphDraftChange: (targetID: string) => void;
  onSaveSelectedGuest: () => void;
  onClearSelectedGuest: () => void;
  onOpenGuest: () => void;
  onOpenLinkedHost: () => void;
};

export function ClusterTopologyGraphView({
  flowModel,
  onGraphNodeClick,
  onGraphNodeDoubleClick,
  onGraphEdgeClick,
  onGraphPaneClick,
  selectedGraphGuest,
  selectedGraphGuestMapping,
  selectedGraphLinkedHost,
  selectedGraphDraftTargetID,
  selectedGraphSaving,
  selectedGraphCanSaveMapping,
  selectedGraphShowNameSuggestion,
  selectedGraphError,
  apiHosts,
  loadingGuestLinks,
  autoLinkingGuestID,
  onSelectedGraphDraftChange,
  onSaveSelectedGuest,
  onClearSelectedGuest,
  onOpenGuest,
  onOpenLinkedHost,
}: ClusterTopologyGraphViewProps) {
  const t = useTranslations("devices-proxmox.topology");
  const linking = loadingGuestLinks || autoLinkingGuestID != null;
  return (
    <>
      <div className="di-ct-lanes" aria-hidden="true">
        <span>{t("lanes.nodes")}</span>
        <span>{t("lanes.guests")}</span>
        <span>{t("lanes.hosts")}</span>
      </div>
      <div className="di-sheet flush di-ct-graph">
        <ReactFlow
          nodes={flowModel.nodes}
          edges={flowModel.edges}
          fitView
          fitViewOptions={{ padding: 0.14 }}
          minZoom={0.35}
          maxZoom={1.8}
          onNodeClick={onGraphNodeClick}
          onNodeDoubleClick={onGraphNodeDoubleClick}
          onEdgeClick={onGraphEdgeClick}
          onPaneClick={onGraphPaneClick}
          nodesDraggable={false}
          nodesConnectable={false}
          proOptions={{ hideAttribution: true }}
        >
          <Controls showInteractive={false} position="bottom-right" />
          <Background color="rgba(128, 128, 128, 0.22)" gap={22} size={1} />
        </ReactFlow>
      </div>
      {selectedGraphGuest ? (
        <div className="di-sheet pad di-ct-editor">
          <div className="di-main">
            <span className="di-nm">{selectedGraphGuest.name}</span>
            <span className="di-grey">
              {[
                friendlyTypeLabel(selectedGraphGuest.type),
                selectedGraphGuestMapping
                  ? (selectedGraphGuestMapping.metadata?.binding === "auto" ? t("autoLinked") : t("manualLinked"))
                  : t("notLinked"),
              ].join(" · ")}
            </span>
          </div>
          <ClusterGuestLinkEditor
            apiHosts={apiHosts}
            draftTargetID={selectedGraphDraftTargetID}
            saving={selectedGraphSaving}
            canSave={selectedGraphCanSaveMapping}
            linked={!!selectedGraphGuestMapping}
            nameMatch={selectedGraphShowNameSuggestion}
            error={selectedGraphError}
            onDraftChange={onSelectedGraphDraftChange}
            onSave={onSaveSelectedGuest}
            onClear={onClearSelectedGuest}
            extra={
              <>
                <button type="button" className="di-btn sm" onClick={onOpenGuest}>{t("openGuest")}</button>
                {selectedGraphLinkedHost ? (
                  <button type="button" className="di-btn sm" onClick={onOpenLinkedHost}>{t("openHost")}</button>
                ) : null}
              </>
            }
          />
        </div>
      ) : (
        <p className="di-note">{linking ? t("linking") : t("editorHint")}</p>
      )}
    </>
  );
}

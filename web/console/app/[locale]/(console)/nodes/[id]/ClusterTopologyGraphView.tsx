"use client";

import { type MouseEvent } from "react";
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type Edge,
  type Node,
} from "@xyflow/react";
import { Select } from "../../../../components/ui/Input";
import type { Asset } from "../../../../console/models";
import { friendlyTypeLabel } from "../../../../console/taxonomy";
import { sourceBadgeLabel } from "./clusterTopologyUtils";
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
  return (
    <div className="space-y-2">
      <div className="grid grid-cols-3 gap-2 text-[10px] uppercase tracking-wide text-[var(--muted)]">
        <span>Proxmox Nodes</span>
        <span>Guests</span>
        <span>API Hosts</span>
      </div>
      <div className="flex flex-wrap items-center gap-1.5 text-[10px]">
        <span className="rounded bg-[var(--accent-subtle)] px-1.5 py-0.5 text-[var(--accent-text)]">manual edge</span>
        <span className="rounded bg-[var(--ok-glow)] px-1.5 py-0.5 text-[var(--ok)]">auto edge</span>
        <span className="rounded bg-[var(--bad-glow)] px-1.5 py-0.5 text-[var(--bad)]">missing target edge</span>
      </div>
      <div className="h-[560px] rounded-lg border border-[var(--line)] bg-[var(--surface)]/35">
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
          <MiniMap
            position="bottom-left"
            pannable
            zoomable
            className="rounded-md border border-[var(--line)]"
          />
          <Controls showInteractive={false} position="bottom-right" />
          <Background color="rgba(255, 255, 255, 0.08)" gap={22} size={1} />
        </ReactFlow>
      </div>
      {selectedGraphGuest ? (
        <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)]/55 p-3 space-y-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="min-w-0">
              <p className="text-xs text-[var(--muted)]">Graph mapping editor</p>
              <p className="text-sm font-medium text-[var(--text)] truncate">{selectedGraphGuest.name}</p>
            </div>
            <div className="flex flex-wrap items-center gap-1">
              <span className="rounded bg-[var(--hover)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]">
                {friendlyTypeLabel(selectedGraphGuest.type)}
              </span>
              {selectedGraphGuestMapping ? (
                <span className="rounded bg-[var(--accent-subtle)] px-1.5 py-0.5 text-[10px] text-[var(--accent-text)]">
                  {selectedGraphGuestMapping.metadata?.binding === "auto" ? "auto link" : "manual link"}
                </span>
              ) : (
                <span className="rounded bg-[var(--warn-glow)] px-1.5 py-0.5 text-[10px] text-[var(--warn)]">
                  not linked
                </span>
              )}
            </div>
          </div>

          <div className="space-y-1.5">
            <Select
              value={selectedGraphDraftTargetID}
              disabled={apiHosts.length === 0 || selectedGraphSaving}
              onChange={(event) => { onSelectedGraphDraftChange(event.target.value); }}
              className="h-8 w-full min-w-0 px-2.5 py-1 text-xs"
            >
              <option value="">
                {apiHosts.length > 0 ? "Select API host..." : "No API hosts"}
              </option>
              {apiHosts.map((host) => (
                <option key={host.id} value={host.id}>
                  {host.name} ({sourceBadgeLabel(host.source)})
                </option>
              ))}
            </Select>

            <div className="flex flex-wrap items-center gap-1.5">
              <button
                type="button"
                disabled={selectedGraphSaving || !selectedGraphCanSaveMapping}
                onClick={onSaveSelectedGuest}
                className="h-7 px-2.5 rounded border border-[var(--line)] text-[10px] font-medium text-[var(--text)] hover:bg-[var(--hover)] disabled:opacity-40 disabled:pointer-events-none"
              >
                {selectedGraphSaving ? "Saving..." : selectedGraphGuestMapping ? "Update link" : "Link"}
              </button>
              {selectedGraphGuestMapping ? (
                <button
                  type="button"
                  disabled={selectedGraphSaving}
                  onClick={onClearSelectedGuest}
                  className="h-7 px-2.5 rounded border border-[var(--line)] text-[10px] font-medium text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] disabled:opacity-40 disabled:pointer-events-none"
                >
                  Clear
                </button>
              ) : null}
              <button
                type="button"
                onClick={onOpenGuest}
                className="h-7 px-2.5 rounded border border-[var(--line)] text-[10px] font-medium text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)]"
              >
                Open guest
              </button>
              {selectedGraphLinkedHost ? (
                <button
                  type="button"
                  onClick={onOpenLinkedHost}
                  className="h-7 px-2.5 rounded border border-[var(--line)] text-[10px] font-medium text-[var(--accent-text)] hover:bg-[var(--accent-subtle)]"
                >
                  Open linked host
                </button>
              ) : null}
              {selectedGraphShowNameSuggestion ? (
                <span className="text-[10px] text-[var(--muted)]">name match</span>
              ) : null}
            </div>
          </div>

          {selectedGraphError ? (
            <p className="text-[10px] text-[var(--bad)]">{selectedGraphError}</p>
          ) : null}
        </div>
      ) : (
        <p className="text-[10px] text-[var(--muted)]">
          Click a guest node or mapping edge to edit links here. Click API host nodes to open details. Double-click a guest node to open details.
        </p>
      )}
      {(loadingGuestLinks || autoLinkingGuestID != null) && !selectedGraphGuest ? (
        <p className="text-[10px] text-[var(--muted)]">syncing links...</p>
      ) : null}
    </div>
  );
}

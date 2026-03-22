"use client";

import type { ClusterStatusEntry, TopologyView } from "./clusterTopologyTypes";

type ClusterTopologyHeaderProps = {
  topologyView: TopologyView;
  onTopologyViewChange: (view: TopologyView) => void;
  clusterEntry?: ClusterStatusEntry;
};

export function ClusterTopologyHeader({
  topologyView,
  onTopologyViewChange,
  clusterEntry,
}: ClusterTopologyHeaderProps) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-2">
      <h2 className="text-sm font-medium text-[var(--text)]">Cluster Topology</h2>
      <div className="flex flex-wrap items-center gap-2">
        <div className="inline-flex rounded-lg border border-[var(--line)] p-0.5">
          <button
            type="button"
            onClick={() => { onTopologyViewChange("graph"); }}
            className={`rounded-md px-2 py-1 text-[10px] font-medium transition-colors ${
              topologyView === "graph"
                ? "bg-[var(--accent)] text-[var(--accent-contrast)]"
                : "text-[var(--muted)] hover:bg-[var(--hover)] hover:text-[var(--text)]"
            }`}
          >
            Graph
          </button>
          <button
            type="button"
            onClick={() => { onTopologyViewChange("list"); }}
            className={`rounded-md px-2 py-1 text-[10px] font-medium transition-colors ${
              topologyView === "list"
                ? "bg-[var(--accent)] text-[var(--accent-contrast)]"
                : "text-[var(--muted)] hover:bg-[var(--hover)] hover:text-[var(--text)]"
            }`}
          >
            List
          </button>
        </div>
        {clusterEntry ? (
          <>
            {clusterEntry.nodes != null ? (
              <span className="text-[10px] px-1.5 py-0.5 rounded-lg border border-[var(--line)] text-[var(--muted)]">
                {clusterEntry.nodes} node{clusterEntry.nodes !== 1 ? "s" : ""}
              </span>
            ) : null}
            {clusterEntry.quorate != null ? (
              <span className={`text-[10px] px-1.5 py-0.5 rounded-lg border border-[var(--line)] ${
                clusterEntry.quorate === 1 ? "text-[var(--ok)] border-[var(--ok)]/30" : "text-[var(--bad)] border-[var(--bad)]/30"
              }`}>
                {clusterEntry.quorate === 1 ? "Quorate" : "No Quorum"}
              </span>
            ) : null}
            {clusterEntry.version != null ? (
              <span className="text-[10px] px-1.5 py-0.5 rounded-lg border border-[var(--line)] text-[var(--muted)]">
                v{clusterEntry.version}
              </span>
            ) : null}
          </>
        ) : null}
      </div>
    </div>
  );
}

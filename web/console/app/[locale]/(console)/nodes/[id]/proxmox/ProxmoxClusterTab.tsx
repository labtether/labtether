"use client";

import { Badge } from "../../../../../components/ui/Badge";
import { Card } from "../../../../../components/ui/Card";
import type { ClusterStatusEntry } from "../nodeDetailTypes";

type Props = {
  clusterStatus: ClusterStatusEntry[];
};

export function ProxmoxClusterTab({ clusterStatus }: Props) {
  const clusterEntry = clusterStatus.find((e) => e.type === "cluster");
  const nodes = clusterStatus.filter((e) => e.type === "node");
  const onlineCount = nodes.filter((n) => n.online === 1).length;
  const offlineCount = nodes.length - onlineCount;

  return (
    <div className="space-y-4">
      {clusterEntry ? (
        <Card>
          <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Cluster</h2>
          <dl className="grid grid-cols-2 gap-x-6 gap-y-1.5">
            <div>
              <dt className="text-xs text-[var(--muted)]">Name</dt>
              <dd className="text-xs text-[var(--text)]">{clusterEntry.name || "unknown"}</dd>
            </div>
            <div>
              <dt className="text-xs text-[var(--muted)]">Quorum</dt>
              <dd className="text-xs">
                <span
                  className={
                    clusterEntry.quorate === 1
                      ? "text-[var(--ok)]"
                      : "text-[var(--bad)]"
                  }
                >
                  {clusterEntry.quorate === 1 ? "Quorate" : "No Quorum"}
                </span>
              </dd>
            </div>
            <div>
              <dt className="text-xs text-[var(--muted)]">Node Count</dt>
              <dd className="text-xs text-[var(--text)]">
                {clusterEntry.nodes != null ? String(clusterEntry.nodes) : nodes.length > 0 ? String(nodes.length) : "-"}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-[var(--muted)]">Version</dt>
              <dd className="text-xs text-[var(--text)]">
                {clusterEntry.version != null ? String(clusterEntry.version) : "-"}
              </dd>
            </div>
          </dl>
        </Card>
      ) : null}

      <Card>
        <div className="mb-3 flex items-center gap-3">
          <h2 className="text-sm font-medium text-[var(--text)]">
            Nodes{nodes.length > 0 ? ` (${nodes.length})` : ""}
          </h2>
          {nodes.length > 0 ? (
            <div className="ml-auto flex items-center gap-3 text-xs">
              <span className="text-[var(--ok)]">{onlineCount} online</span>
              {offlineCount > 0 ? (
                <span className="text-[var(--bad)]">{offlineCount} offline</span>
              ) : null}
            </div>
          ) : null}
        </div>
        {nodes.length > 0 ? (
          <ul className="divide-y divide-[var(--line)]">
            {nodes.map((node, idx) => (
              <li
                key={`${node.name ?? "node"}-${idx}`}
                className="flex items-center justify-between gap-3 py-2.5"
              >
                <div>
                  <span className="text-sm font-medium text-[var(--text)]">
                    {node.name || "unknown"}
                  </span>
                  <code className="block text-xs text-[var(--muted)]">
                    {node.ip || ""}
                    {node.nodeid != null ? ` · node ${node.nodeid}` : ""}
                  </code>
                </div>
                <div className="flex items-center gap-2">
                  <Badge status={node.online === 1 ? "ok" : "bad"} />
                  {node.local === 1 ? (
                    <span className="rounded-lg border border-[var(--line)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]">
                      local
                    </span>
                  ) : null}
                  {node.level ? (
                    <span className="rounded-lg border border-[var(--line)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]">
                      {node.level}
                    </span>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-xs text-[var(--muted)]">No cluster nodes returned.</p>
        )}
      </Card>
    </div>
  );
}

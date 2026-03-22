"use client";

import { useMemo } from "react";

import { formatBytes as formatByteTotal } from "../../../../console/formatters";
import type { NetworkInterfaceInfo } from "../../../../hooks/useNetworkInterfaces";
import { Card } from "../../../../components/ui/Card";
import {
  formatInterfaceAddress,
  interfaceTraffic,
  toneStyle,
} from "./systemPanelModel";

export function SystemPanelNetworkInterfacePreview({
  interfaces,
  loading,
  error,
}: {
  interfaces: NetworkInterfaceInfo[];
  loading: boolean;
  error: string | null;
}) {
  const topInterfaces = useMemo(() => {
    return [...interfaces]
      .sort((left, right) => interfaceTraffic(right) - interfaceTraffic(left))
      .slice(0, 4);
  }, [interfaces]);

  return (
    <Card>
      <h4 className="text-sm font-medium text-[var(--text)]">Top Interface Activity</h4>
      <p className="mt-1 text-xs text-[var(--muted)]">
        Inline preview of the busiest reported links so you can stay in the network drilldown longer before opening Interfaces.
      </p>

      {loading && interfaces.length === 0 ? (
        <div className="mt-4 rounded-lg border border-dashed border-[var(--line)] p-4 text-sm text-[var(--muted)]">
          Loading interface details...
        </div>
      ) : error ? (
        <div className="mt-4 rounded-lg border border-[var(--bad)]/30 bg-[var(--bad-glow)] p-4 text-sm text-[var(--bad)]">
          {error}
        </div>
      ) : topInterfaces.length > 0 ? (
        <div className="mt-4 space-y-3">
          {topInterfaces.map((entry) => {
            const state = entry.state.trim().toLowerCase();
            const isUp = state === "up" || state === "active";
            return (
              <div
                key={entry.name}
                className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2"
              >
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <div className="flex items-center gap-2">
                      <p className="text-sm font-medium text-[var(--text)]">
                        {entry.name || "Unnamed interface"}
                      </p>
                      <span
                        className="inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-medium"
                        style={toneStyle(isUp ? "ok" : "warn")}
                      >
                        {entry.state || "unknown"}
                      </span>
                    </div>
                    <p className="mt-1 text-xs tabular-nums text-[var(--muted)]">
                      {formatInterfaceAddress(entry)}
                    </p>
                  </div>
                  <div className="text-right text-xs tabular-nums text-[var(--muted)]">
                    <div>RX {formatByteTotal(Math.max(0, entry.rx_bytes))}</div>
                    <div>TX {formatByteTotal(Math.max(0, entry.tx_bytes))}</div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <div className="mt-4 rounded-lg border border-dashed border-[var(--line)] p-4 text-sm text-[var(--muted)]">
          No interface details were returned for this node.
        </div>
      )}
    </Card>
  );
}

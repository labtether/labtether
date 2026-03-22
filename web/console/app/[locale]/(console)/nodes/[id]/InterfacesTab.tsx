"use client";
import { useEffect, useMemo } from "react";
import type { NetworkInterfaceInfo } from "../../../../hooks/useNetworkInterfaces";
import { useNetworkInterfaces } from "../../../../hooks/useNetworkInterfaces";

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) {
    return "-";
  }
  if (bytes < 1) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const index = Math.min(Math.max(0, Math.floor(Math.log(bytes) / Math.log(1024))), units.length - 1);
  const value = bytes / Math.pow(1024, index);
  return `${value.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function stateBadgeTone(state: string): { background: string; color: string } {
  const normalized = state.trim().toLowerCase();
  if (normalized === "up" || normalized === "active") {
    return { background: "var(--ok-glow)", color: "var(--ok)" };
  }
  if (normalized === "down" || normalized === "inactive") {
    return { background: "var(--bad-glow)", color: "var(--bad)" };
  }
  return { background: "var(--surface)", color: "var(--muted)" };
}

function sortInterfaces(entries: NetworkInterfaceInfo[]): NetworkInterfaceInfo[] {
  return [...entries].sort((left, right) => {
    const leftActive = left.state.trim().toLowerCase() === "up" || left.state.trim().toLowerCase() === "active";
    const rightActive = right.state.trim().toLowerCase() === "up" || right.state.trim().toLowerCase() === "active";
    if (leftActive !== rightActive) {
      return leftActive ? -1 : 1;
    }
    return left.name.localeCompare(right.name);
  });
}

export function InterfacesTab({ nodeId }: { nodeId: string }) {
  const { interfaces, loading, error, refresh } = useNetworkInterfaces(nodeId);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const sortedInterfaces = useMemo(() => sortInterfaces(interfaces), [interfaces]);

  return (
    <div
      style={{
        background: "var(--panel)",
        border: "1px solid var(--line)",
        borderRadius: "8px",
        padding: "16px",
        marginBottom: "16px",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "12px", gap: "12px", flexWrap: "wrap" }}>
        <h2 style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)", margin: 0 }}>Network Interfaces</h2>
        <button
          onClick={() => void refresh()}
          disabled={loading}
          style={{
            padding: "5px 10px",
            fontSize: "11px",
            borderRadius: "6px",
            border: "1px solid var(--line)",
            cursor: loading ? "default" : "pointer",
            background: "transparent",
            color: "var(--muted)",
            opacity: loading ? 0.5 : 1,
            whiteSpace: "nowrap",
          }}
        >
          {loading ? "Loading..." : "Refresh"}
        </button>
      </div>

      {loading && interfaces.length === 0 ? (
        <p style={{ fontSize: "14px", color: "var(--muted)" }}>Loading network interfaces...</p>
      ) : error ? (
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "32px 0", gap: "8px" }}>
          <p style={{ fontSize: "12px", color: "var(--bad)" }}>{error}</p>
          <button
            onClick={() => void refresh()}
            style={{
              padding: "6px 12px",
              fontSize: "12px",
              borderRadius: "6px",
              border: "1px solid var(--line)",
              cursor: "pointer",
              background: "transparent",
              color: "var(--text)",
            }}
          >
            Retry
          </button>
        </div>
      ) : sortedInterfaces.length === 0 ? (
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "48px 0", gap: "8px" }}>
          <p style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)" }}>No interfaces returned</p>
          <p style={{ fontSize: "12px", color: "var(--muted)", textAlign: "center", maxWidth: "360px" }}>
            The agent did not report any network interfaces for this node.
          </p>
        </div>
      ) : (
        <div style={{ overflowX: "auto" }}>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "12px" }}>
            <thead>
              <tr style={{ borderBottom: "1px solid var(--line)" }}>
                {["Interface", "State", "MAC", "MTU", "IPs", "RX", "TX"].map((column) => (
                  <th
                    key={column}
                    style={{
                      padding: "6px 8px",
                      textAlign: "left",
                      fontSize: "10px",
                      fontWeight: 500,
                      color: "var(--muted)",
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {column}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {sortedInterfaces.map((entry) => {
                const tone = stateBadgeTone(entry.state);
                return (
                  <tr
                    key={entry.name}
                    style={{ borderBottom: "1px solid var(--line)" }}
                    onMouseEnter={(event) => {
                      (event.currentTarget as HTMLTableRowElement).style.background = "var(--hover)";
                    }}
                    onMouseLeave={(event) => {
                      (event.currentTarget as HTMLTableRowElement).style.background = "transparent";
                    }}
                  >
                    <td style={{ padding: "6px 8px", color: "var(--text)", fontWeight: 500, whiteSpace: "nowrap" }}>
                      {entry.name || "-"}
                    </td>
                    <td style={{ padding: "6px 8px" }}>
                      <span
                        style={{
                          display: "inline-block",
                          padding: "2px 8px",
                          borderRadius: "4px",
                          fontSize: "10px",
                          fontWeight: 500,
                          background: tone.background,
                          color: tone.color,
                          whiteSpace: "nowrap",
                        }}
                      >
                        {entry.state || "unknown"}
                      </span>
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap" }}>
                      {entry.mac || "-"}
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap" }}>
                      {Number.isFinite(entry.mtu) && entry.mtu > 0 ? entry.mtu : "-"}
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", fontVariantNumeric: "tabular-nums", maxWidth: "260px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {entry.ips?.length ? entry.ips.join(", ") : "-"}
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap" }}>
                      <div>{formatBytes(entry.rx_bytes)}</div>
                      <div style={{ fontSize: "10px", color: "var(--muted)" }}>{(entry.rx_packets ?? 0).toLocaleString()} packets</div>
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap" }}>
                      <div>{formatBytes(entry.tx_bytes)}</div>
                      <div style={{ fontSize: "10px", color: "var(--muted)" }}>{(entry.tx_packets ?? 0).toLocaleString()} packets</div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

"use client";
import { useEffect, useState } from "react";
import { useProcesses } from "../../../../hooks/useProcesses";

function formatRSS(bytes: number): string {
  if (bytes >= 1024 * 1024 * 1024) {
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
  }
  if (bytes >= 1024 * 1024) {
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }
  if (bytes >= 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  return `${bytes} B`;
}

export function ProcessesTab({ nodeId }: { nodeId: string }) {
  const { processes, loading, error, refresh } = useProcesses(nodeId);
  const [sortBy, setSortBy] = useState<"cpu" | "memory">("cpu");

  useEffect(() => {
    void refresh(sortBy);
  }, [refresh, sortBy]);

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
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "12px" }}>
        <h2 style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)", margin: 0 }}>Processes</h2>
        <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
          <span style={{ fontSize: "11px", color: "var(--muted)" }}>Sort by:</span>
          {(["cpu", "memory"] as const).map((s) => (
            <button
              key={s}
              onClick={() => setSortBy(s)}
              style={{
                padding: "4px 10px",
                fontSize: "11px",
                borderRadius: "6px",
                border: "none",
                cursor: "pointer",
                background: sortBy === s ? "var(--accent)" : "var(--surface)",
                color: sortBy === s ? "var(--accent-contrast)" : "var(--muted)",
                transition: "background 150ms, color 150ms",
              }}
            >
              {s === "cpu" ? "CPU" : "Memory"}
            </button>
          ))}
          <button
            onClick={() => void refresh(sortBy)}
            disabled={loading}
            style={{
              padding: "4px 10px",
              fontSize: "11px",
              borderRadius: "6px",
              border: "1px solid var(--line)",
              cursor: loading ? "default" : "pointer",
              background: "transparent",
              color: "var(--muted)",
              transition: "color 150ms",
              opacity: loading ? 0.5 : 1,
            }}
          >
            {loading ? "Loading..." : "Refresh"}
          </button>
        </div>
      </div>

      {loading && processes.length === 0 ? (
        <p style={{ fontSize: "14px", color: "var(--muted)" }}>Loading processes...</p>
      ) : error ? (
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "32px 0", gap: "8px" }}>
          <p style={{ fontSize: "12px", color: "var(--bad)" }}>{error}</p>
          <button
            onClick={() => void refresh(sortBy)}
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
      ) : processes.length === 0 ? (
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "48px 0", gap: "8px" }}>
          <p style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)" }}>No processes returned</p>
          <p style={{ fontSize: "12px", color: "var(--muted)", textAlign: "center", maxWidth: "320px" }}>
            The agent did not return any process data. Make sure the agent is connected and supports process listing.
          </p>
        </div>
      ) : (
        <div style={{ overflowX: "auto" }}>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "12px" }}>
            <thead>
              <tr style={{ borderBottom: "1px solid var(--line)" }}>
                {["PID", "Name", "User", "CPU %", "Mem %", "RSS", "Command"].map((col) => (
                  <th
                    key={col}
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
                    {col}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {processes.map((proc) => (
                <tr
                  key={proc.pid}
                  style={{ borderBottom: "1px solid var(--line)" }}
                  onMouseEnter={(e) => {
                    (e.currentTarget as HTMLTableRowElement).style.background = "var(--hover)";
                  }}
                  onMouseLeave={(e) => {
                    (e.currentTarget as HTMLTableRowElement).style.background = "transparent";
                  }}
                >
                  <td style={{ padding: "6px 8px", color: "var(--muted)", fontVariantNumeric: "tabular-nums" }}>
                    {proc.pid}
                  </td>
                  <td style={{ padding: "6px 8px", color: "var(--text)", fontWeight: 500, maxWidth: "160px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    {proc.name}
                  </td>
                  <td style={{ padding: "6px 8px", color: "var(--muted)" }}>
                    {proc.user}
                  </td>
                  <td style={{ padding: "6px 8px", color: proc.cpu_pct > 50 ? "var(--bad)" : proc.cpu_pct > 20 ? "var(--warn)" : "var(--text)", fontVariantNumeric: "tabular-nums" }}>
                    {proc.cpu_pct.toFixed(1)}%
                  </td>
                  <td style={{ padding: "6px 8px", color: proc.mem_pct > 50 ? "var(--bad)" : proc.mem_pct > 20 ? "var(--warn)" : "var(--text)", fontVariantNumeric: "tabular-nums" }}>
                    {proc.mem_pct.toFixed(1)}%
                  </td>
                  <td style={{ padding: "6px 8px", color: "var(--muted)", fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap" }}>
                    {formatRSS(proc.mem_rss)}
                  </td>
                  <td style={{ padding: "6px 8px", color: "var(--muted)", maxWidth: "300px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    <code style={{ fontSize: "11px" }}>{proc.command}</code>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

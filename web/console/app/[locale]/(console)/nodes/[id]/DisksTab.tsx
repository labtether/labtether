"use client";
import { useEffect, useMemo } from "react";
import { useDisks } from "../../../../hooks/useDisks";

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

function normalizedPercent(value: number): number {
  if (!Number.isFinite(value)) {
    return 0;
  }
  return Math.min(Math.max(value, 0), 100);
}

function percentColor(value: number): string {
  if (value >= 90) return "var(--bad)";
  if (value >= 75) return "var(--warn)";
  return "var(--ok)";
}

export function DisksTab({ nodeId }: { nodeId: string }) {
  const { mounts, loading, error, refresh } = useDisks(nodeId);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const sortedMounts = useMemo(
    () => [...mounts].sort((left, right) => normalizedPercent(right.use_pct) - normalizedPercent(left.use_pct)),
    [mounts],
  );

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
        <h2 style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)", margin: 0 }}>Disks</h2>
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

      {loading && mounts.length === 0 ? (
        <p style={{ fontSize: "14px", color: "var(--muted)" }}>Loading disks...</p>
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
      ) : sortedMounts.length === 0 ? (
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "48px 0", gap: "8px" }}>
          <p style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)" }}>No disks returned</p>
          <p style={{ fontSize: "12px", color: "var(--muted)", textAlign: "center", maxWidth: "360px" }}>
            The agent did not return any mounted filesystems for this node.
          </p>
        </div>
      ) : (
        <div style={{ overflowX: "auto" }}>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "12px" }}>
            <thead>
              <tr style={{ borderBottom: "1px solid var(--line)" }}>
                {["Device", "Mount", "FS", "Used", "Available", "Total", "Usage"].map((column) => (
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
              {sortedMounts.map((mount) => {
                const usage = normalizedPercent(mount.use_pct);
                const usageTone = percentColor(usage);
                return (
                  <tr
                    key={`${mount.mount_point}-${mount.device}`}
                    style={{ borderBottom: "1px solid var(--line)" }}
                    onMouseEnter={(event) => {
                      (event.currentTarget as HTMLTableRowElement).style.background = "var(--hover)";
                    }}
                    onMouseLeave={(event) => {
                      (event.currentTarget as HTMLTableRowElement).style.background = "transparent";
                    }}
                  >
                    <td style={{ padding: "6px 8px", color: "var(--muted)", maxWidth: "220px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {mount.device || "-"}
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--text)", fontWeight: 500, maxWidth: "220px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {mount.mount_point || "-"}
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", whiteSpace: "nowrap" }}>
                      {mount.fs_type || "-"}
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap" }}>
                      {formatBytes(mount.used)}
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap" }}>
                      {formatBytes(mount.available)}
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap" }}>
                      {formatBytes(mount.total)}
                    </td>
                    <td style={{ padding: "6px 8px", minWidth: "170px" }}>
                      <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                        <div
                          style={{
                            flex: 1,
                            height: "6px",
                            borderRadius: "999px",
                            background: "var(--surface)",
                            overflow: "hidden",
                          }}
                        >
                          <div
                            style={{
                              width: `${usage}%`,
                              height: "100%",
                              background: usageTone,
                            }}
                          />
                        </div>
                        <span style={{ fontVariantNumeric: "tabular-nums", color: usageTone, minWidth: "46px", textAlign: "right" }}>
                          {usage.toFixed(1)}%
                        </span>
                      </div>
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

"use client";
import { useEffect, useMemo } from "react";
import { useCron } from "../../../../hooks/useCron";

function formatTimestamp(raw: string | undefined): string {
  if (!raw || raw.trim() === "") {
    return "-";
  }
  const parsed = new Date(raw);
  if (Number.isNaN(parsed.getTime())) {
    return raw;
  }
  return parsed.toLocaleString();
}

function sourceLabel(source: string): string {
  const normalized = source.trim().toLowerCase();
  if (normalized === "systemd-timer") {
    return "Timer";
  }
  if (normalized === "launchd") {
    return "Launchd";
  }
  if (normalized === "crontab") {
    return "Cron";
  }
  return source || "Unknown";
}

function sourceTone(source: string): { background: string; color: string } {
  const normalized = source.trim().toLowerCase();
  if (normalized === "systemd-timer") {
    return { background: "rgba(59, 130, 246, 0.15)", color: "#3b82f6" };
  }
  if (normalized === "launchd") {
    return { background: "rgba(139, 92, 246, 0.15)", color: "#8b5cf6" };
  }
  if (normalized === "crontab") {
    return { background: "var(--ok-glow)", color: "var(--ok)" };
  }
  return { background: "var(--surface)", color: "var(--muted)" };
}

export function CronTab({ nodeId }: { nodeId: string }) {
  const { entries, loading, error, refresh } = useCron(nodeId);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const sortedEntries = useMemo(() => {
    return [...entries].sort((left, right) => {
      const source = left.source.localeCompare(right.source);
      if (source !== 0) return source;
      const user = left.user.localeCompare(right.user);
      if (user !== 0) return user;
      return left.schedule.localeCompare(right.schedule);
    });
  }, [entries]);

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
        <h2 style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)", margin: 0 }}>Cron and Timers</h2>
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

      {loading && entries.length === 0 ? (
        <p style={{ fontSize: "14px", color: "var(--muted)" }}>Loading cron and timer entries...</p>
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
      ) : sortedEntries.length === 0 ? (
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "48px 0", gap: "8px" }}>
          <p style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)" }}>No cron or timer entries returned</p>
          <p style={{ fontSize: "12px", color: "var(--muted)", textAlign: "center", maxWidth: "360px" }}>
            The agent did not report scheduled tasks (launchd jobs, crontab jobs, or systemd timers) for this node.
          </p>
        </div>
      ) : (
        <div style={{ overflowX: "auto" }}>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "12px" }}>
            <thead>
              <tr style={{ borderBottom: "1px solid var(--line)" }}>
                {["Type", "User", "Schedule", "Command", "Next Run", "Last Run"].map((column) => (
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
              {sortedEntries.map((entry, index) => {
                const tone = sourceTone(entry.source);
                return (
                  <tr
                    key={`${entry.source}-${entry.user}-${entry.schedule}-${index}`}
                    style={{ borderBottom: "1px solid var(--line)" }}
                    onMouseEnter={(event) => {
                      (event.currentTarget as HTMLTableRowElement).style.background = "var(--hover)";
                    }}
                    onMouseLeave={(event) => {
                      (event.currentTarget as HTMLTableRowElement).style.background = "transparent";
                    }}
                  >
                    <td style={{ padding: "6px 8px", whiteSpace: "nowrap" }}>
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
                        {sourceLabel(entry.source)}
                      </span>
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", whiteSpace: "nowrap" }}>
                      {entry.user || "-"}
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--text)", fontWeight: 500, whiteSpace: "nowrap" }}>
                      {entry.schedule || "-"}
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", maxWidth: "360px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      <code style={{ fontSize: "11px" }}>{entry.command || "-"}</code>
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", whiteSpace: "nowrap" }}>
                      {formatTimestamp(entry.next_run)}
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", whiteSpace: "nowrap" }}>
                      {formatTimestamp(entry.last_run)}
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

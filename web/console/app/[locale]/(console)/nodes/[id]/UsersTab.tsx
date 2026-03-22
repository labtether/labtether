"use client";
import { useEffect, useMemo } from "react";
import { useUsers } from "../../../../hooks/useUsers";

function formatLoginTime(value: string): string {
  if (!value || value.trim() === "") {
    return "-";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString();
}

export function UsersTab({ nodeId }: { nodeId: string }) {
  const { sessions, loading, error, refresh } = useUsers(nodeId);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const sortedSessions = useMemo(
    () => [...sessions].sort((left, right) => left.username.localeCompare(right.username)),
    [sessions],
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
        <h2 style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)", margin: 0 }}>Active Users</h2>
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

      {loading && sessions.length === 0 ? (
        <p style={{ fontSize: "14px", color: "var(--muted)" }}>Loading active user sessions...</p>
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
      ) : sortedSessions.length === 0 ? (
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "48px 0", gap: "8px" }}>
          <p style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)" }}>No active sessions returned</p>
          <p style={{ fontSize: "12px", color: "var(--muted)", textAlign: "center", maxWidth: "360px" }}>
            The agent did not report any logged-in users on this node.
          </p>
        </div>
      ) : (
        <div style={{ overflowX: "auto" }}>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "12px" }}>
            <thead>
              <tr style={{ borderBottom: "1px solid var(--line)" }}>
                {["User", "Terminal", "Remote Host", "Login Time"].map((column) => (
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
              {sortedSessions.map((session, index) => (
                <tr
                  key={`${session.username}-${session.terminal}-${index}`}
                  style={{ borderBottom: "1px solid var(--line)" }}
                  onMouseEnter={(event) => {
                    (event.currentTarget as HTMLTableRowElement).style.background = "var(--hover)";
                  }}
                  onMouseLeave={(event) => {
                    (event.currentTarget as HTMLTableRowElement).style.background = "transparent";
                  }}
                >
                  <td style={{ padding: "6px 8px", color: "var(--text)", fontWeight: 500, whiteSpace: "nowrap" }}>
                    {session.username || "-"}
                  </td>
                  <td style={{ padding: "6px 8px", color: "var(--muted)", whiteSpace: "nowrap" }}>
                    {session.terminal || "-"}
                  </td>
                  <td style={{ padding: "6px 8px", color: "var(--muted)", maxWidth: "220px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    {session.remote_host || "-"}
                  </td>
                  <td style={{ padding: "6px 8px", color: "var(--muted)", whiteSpace: "nowrap" }}>
                    {formatLoginTime(session.login_time)}
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

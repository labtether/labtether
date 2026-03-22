"use client";
import { useEffect, useState } from "react";
import { useServices } from "../../../../hooks/useServices";
import type { ServiceInfo } from "../../../../hooks/useServices";

function ActiveStateBadge({ state }: { state: string }) {
  const lower = state.toLowerCase();
  let bg = "var(--surface)";
  let color = "var(--muted)";
  if (lower === "active") {
    bg = "var(--ok-glow)";
    color = "var(--ok)";
  } else if (lower === "failed") {
    bg = "var(--bad-glow)";
    color = "var(--bad)";
  } else if (lower === "activating" || lower === "deactivating" || lower === "reloading") {
    bg = "var(--warn-glow)";
    color = "var(--warn)";
  }
  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 8px",
        borderRadius: "4px",
        fontSize: "10px",
        fontWeight: 500,
        background: bg,
        color: color,
        whiteSpace: "nowrap",
      }}
    >
      {state || "unknown"}
    </span>
  );
}

function EnabledBadge({ enabled }: { enabled: string }) {
  const lower = enabled.toLowerCase();
  const isEnabled = lower === "enabled" || lower === "enabled-runtime";
  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 8px",
        borderRadius: "4px",
        fontSize: "10px",
        fontWeight: 500,
        background: isEnabled ? "var(--ok-glow)" : "var(--surface)",
        color: isEnabled ? "var(--ok)" : "var(--muted)",
        whiteSpace: "nowrap",
      }}
    >
      {enabled || "unknown"}
    </span>
  );
}

function ServiceActions({
  service,
  onAction,
  busy,
}: {
  service: ServiceInfo;
  onAction: (action: string) => void;
  busy: boolean;
}) {
  const active = service.active_state.toLowerCase();
  const isActive = active === "active";
  const isFailed = active === "failed";
  const isStopped = active === "inactive" || isFailed;

  return (
    <div style={{ display: "flex", gap: "4px" }}>
      {isStopped ? (
        <button
          disabled={busy}
          onClick={() => onAction("start")}
          style={{
            padding: "3px 8px",
            fontSize: "10px",
            borderRadius: "4px",
            border: "1px solid var(--line)",
            cursor: busy ? "default" : "pointer",
            background: "transparent",
            color: "var(--ok)",
            opacity: busy ? 0.5 : 1,
            whiteSpace: "nowrap",
          }}
        >
          Start
        </button>
      ) : null}
      {isActive ? (
        <button
          disabled={busy}
          onClick={() => onAction("stop")}
          style={{
            padding: "3px 8px",
            fontSize: "10px",
            borderRadius: "4px",
            border: "1px solid var(--line)",
            cursor: busy ? "default" : "pointer",
            background: "transparent",
            color: "var(--bad)",
            opacity: busy ? 0.5 : 1,
            whiteSpace: "nowrap",
          }}
        >
          Stop
        </button>
      ) : null}
      {(isActive || isFailed) ? (
        <button
          disabled={busy}
          onClick={() => onAction("restart")}
          style={{
            padding: "3px 8px",
            fontSize: "10px",
            borderRadius: "4px",
            border: "1px solid var(--line)",
            cursor: busy ? "default" : "pointer",
            background: "transparent",
            color: "var(--muted)",
            opacity: busy ? 0.5 : 1,
            whiteSpace: "nowrap",
          }}
        >
          Restart
        </button>
      ) : null}
    </div>
  );
}

export function ServicesTab({ nodeId }: { nodeId: string }) {
  const { services, loading, error, refresh, performAction } = useServices(nodeId);
  const [filter, setFilter] = useState("");
  const [busyService, setBusyService] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const filtered = services.filter((svc) => {
    if (!filter) return true;
    const q = filter.toLowerCase();
    return (
      svc.name.toLowerCase().includes(q) ||
      svc.description.toLowerCase().includes(q) ||
      svc.active_state.toLowerCase().includes(q)
    );
  });

  const handleAction = async (serviceName: string, action: string) => {
    setBusyService(serviceName);
    setActionError(null);
    try {
      await performAction(serviceName, action);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : `Failed to ${action} service`);
    } finally {
      setBusyService(null);
    }
  };

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
        <h2 style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)", margin: 0 }}>Services</h2>
        <div style={{ display: "flex", alignItems: "center", gap: "8px", flex: 1, minWidth: "200px", maxWidth: "400px" }}>
          <input
            type="text"
            placeholder="Filter services..."
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            style={{
              flex: 1,
              padding: "5px 10px",
              fontSize: "12px",
              borderRadius: "6px",
              border: "1px solid var(--line)",
              background: "transparent",
              color: "var(--text)",
              outline: "none",
            }}
          />
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
      </div>

      {actionError ? (
        <p style={{ fontSize: "12px", color: "var(--bad)", marginBottom: "8px" }}>{actionError}</p>
      ) : null}

      {loading && services.length === 0 ? (
        <p style={{ fontSize: "14px", color: "var(--muted)" }}>Loading services...</p>
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
      ) : services.length === 0 ? (
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "48px 0", gap: "8px" }}>
          <p style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)" }}>No services returned</p>
          <p style={{ fontSize: "12px", color: "var(--muted)", textAlign: "center", maxWidth: "320px" }}>
            The agent did not return any service data. Make sure the agent is connected and supports service listing.
          </p>
        </div>
      ) : (
        <>
          {filter && (
            <p style={{ fontSize: "11px", color: "var(--muted)", marginBottom: "8px" }}>
              Showing {filtered.length} of {services.length} services
            </p>
          )}
          <div style={{ overflowX: "auto" }}>
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "12px" }}>
              <thead>
                <tr style={{ borderBottom: "1px solid var(--line)" }}>
                  {["Name", "Description", "Active", "Sub-State", "Enabled", "Load", "Actions"].map((col) => (
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
                {filtered.map((svc) => (
                  <tr
                    key={svc.name}
                    style={{ borderBottom: "1px solid var(--line)" }}
                    onMouseEnter={(e) => {
                      (e.currentTarget as HTMLTableRowElement).style.background = "var(--hover)";
                    }}
                    onMouseLeave={(e) => {
                      (e.currentTarget as HTMLTableRowElement).style.background = "transparent";
                    }}
                  >
                    <td style={{ padding: "6px 8px", color: "var(--text)", fontWeight: 500, maxWidth: "200px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {svc.name}
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", maxWidth: "240px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {svc.description || "-"}
                    </td>
                    <td style={{ padding: "6px 8px" }}>
                      <ActiveStateBadge state={svc.active_state} />
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", whiteSpace: "nowrap" }}>
                      {svc.sub_state || "-"}
                    </td>
                    <td style={{ padding: "6px 8px" }}>
                      <EnabledBadge enabled={svc.enabled} />
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", whiteSpace: "nowrap" }}>
                      {svc.load_state || "-"}
                    </td>
                    <td style={{ padding: "6px 8px" }}>
                      <ServiceActions
                        service={svc}
                        onAction={(action) => void handleAction(svc.name, action)}
                        busy={busyService === svc.name}
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}

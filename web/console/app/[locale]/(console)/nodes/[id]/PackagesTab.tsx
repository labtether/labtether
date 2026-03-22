"use client";
import { useEffect, useMemo, useState } from "react";
import { usePackages } from "../../../../hooks/usePackages";

type PackagesTabProps = {
  nodeId: string;
  backend?: string;
};

export function PackagesTab({ nodeId, backend = "" }: PackagesTabProps) {
  const { packages, loading, error, refresh, performAction } = usePackages(nodeId);
  const [filter, setFilter] = useState("");
  const [action, setAction] = useState<"install" | "remove" | "upgrade">("install");
  const [packageInput, setPackageInput] = useState("");
  const [actionBusy, setActionBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionOutput, setActionOutput] = useState<string | null>(null);
  const [rebootRequired, setRebootRequired] = useState(false);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const filtered = useMemo(() => {
    const sorted = [...packages].sort((a, b) => a.name.localeCompare(b.name));
    if (!filter) return sorted;
    const q = filter.toLowerCase();
    return sorted.filter(
      (pkg) =>
        pkg.name.toLowerCase().includes(q) ||
        pkg.version.toLowerCase().includes(q) ||
        pkg.status.toLowerCase().includes(q)
    );
  }, [packages, filter]);

  const parsePackageInput = () => packageInput
    .split(/[,\s]+/)
    .map((value) => value.trim())
    .filter((value) => value.length > 0);

  const backendLabel = backend.trim().toLowerCase() === "brew"
    ? "Homebrew"
    : backend.trim();

  const runAction = async (selectedAction: "install" | "remove" | "upgrade", upgradeAll: boolean) => {
    const parsedPackages = upgradeAll ? [] : parsePackageInput();
    if (!upgradeAll && (selectedAction === "install" || selectedAction === "remove") && parsedPackages.length === 0) {
      setActionError("Enter at least one package name.");
      return;
    }

    setActionBusy(true);
    setActionError(null);
    setActionOutput(null);
    setRebootRequired(false);
    try {
      const result = await performAction(selectedAction, parsedPackages);
      setActionOutput(result.output?.trim() || "Package action completed.");
      setRebootRequired(Boolean(result.reboot_required));
      if (selectedAction !== "upgrade") {
        setPackageInput("");
      }
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Package action failed.");
    } finally {
      setActionBusy(false);
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
        <h2 style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)", margin: 0 }}>Packages</h2>
        <div style={{ display: "flex", alignItems: "center", gap: "8px", flex: 1, minWidth: "200px", maxWidth: "400px" }}>
          <input
            type="text"
            placeholder="Filter packages..."
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

      <div style={{ display: "grid", gridTemplateColumns: "minmax(180px, 1fr) 130px auto auto", gap: "8px", marginBottom: "12px", alignItems: "center" }}>
        <input
          type="text"
          placeholder={action === "upgrade" ? "Optional packages (comma or space separated)" : "Package names (comma or space separated)"}
          value={packageInput}
          onChange={(e) => setPackageInput(e.target.value)}
          style={{
            padding: "6px 10px",
            fontSize: "12px",
            borderRadius: "6px",
            border: "1px solid var(--line)",
            background: "transparent",
            color: "var(--text)",
            outline: "none",
            width: "100%",
          }}
        />
        <select
          value={action}
          onChange={(e) => setAction(e.target.value as "install" | "remove" | "upgrade")}
          style={{
            padding: "6px 10px",
            fontSize: "12px",
            borderRadius: "6px",
            border: "1px solid var(--line)",
            background: "var(--surface)",
            color: "var(--text)",
            outline: "none",
          }}
        >
          <option value="install">Install</option>
          <option value="remove">Remove</option>
          <option value="upgrade">Upgrade</option>
        </select>
        <button
          onClick={() => { void runAction(action, false); }}
          disabled={actionBusy}
          style={{
            padding: "6px 12px",
            fontSize: "12px",
            borderRadius: "6px",
            border: "1px solid var(--line)",
            cursor: actionBusy ? "default" : "pointer",
            background: "transparent",
            color: "var(--text)",
            opacity: actionBusy ? 0.5 : 1,
            whiteSpace: "nowrap",
          }}
        >
          {actionBusy ? "Running..." : `${action[0].toUpperCase()}${action.slice(1)}`}
        </button>
        <button
          onClick={() => {
            setAction("upgrade");
            void runAction("upgrade", true);
          }}
          disabled={actionBusy}
          style={{
            padding: "6px 12px",
            fontSize: "12px",
            borderRadius: "6px",
            border: "1px solid var(--line)",
            cursor: actionBusy ? "default" : "pointer",
            background: "transparent",
            color: "var(--muted)",
            opacity: actionBusy ? 0.5 : 1,
            whiteSpace: "nowrap",
          }}
        >
          Upgrade All
        </button>
      </div>

      {actionError ? (
        <p style={{ fontSize: "12px", color: "var(--bad)", marginBottom: "8px" }}>{actionError}</p>
      ) : null}
      {rebootRequired ? (
        <p style={{ fontSize: "12px", color: "var(--warn)", marginBottom: "8px" }}>Reboot required to finish applying package changes.</p>
      ) : null}
      {actionOutput ? (
        <pre
          style={{
            margin: "0 0 12px 0",
            maxHeight: "140px",
            overflow: "auto",
            padding: "8px",
            borderRadius: "6px",
            border: "1px solid var(--line)",
            background: "var(--surface)",
            color: "var(--text)",
            fontSize: "11px",
            whiteSpace: "pre-wrap",
            wordBreak: "break-word",
          }}
        >
          {actionOutput}
        </pre>
      ) : null}

      {loading && packages.length === 0 ? (
        <p style={{ fontSize: "14px", color: "var(--muted)" }}>Loading packages...</p>
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
      ) : packages.length === 0 ? (
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "48px 0", gap: "8px" }}>
          <p style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)" }}>No packages returned</p>
          <p style={{ fontSize: "12px", color: "var(--muted)", textAlign: "center", maxWidth: "320px" }}>
            The agent did not return any package data.
            {backendLabel
              ? ` Make sure ${backendLabel} is available to the agent on this node.`
              : " Make sure package tooling is available to the agent on this node."}
          </p>
        </div>
      ) : (
        <>
          {filter && (
            <p style={{ fontSize: "11px", color: "var(--muted)", marginBottom: "8px" }}>
              Showing {filtered.length} of {packages.length} packages
            </p>
          )}
          <div style={{ overflowX: "auto" }}>
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "12px" }}>
              <thead>
                <tr style={{ borderBottom: "1px solid var(--line)" }}>
                  {["Name", "Version", "Status"].map((col) => (
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
                {filtered.map((pkg) => (
                  <tr
                    key={`${pkg.name}-${pkg.version}`}
                    style={{ borderBottom: "1px solid var(--line)" }}
                    onMouseEnter={(e) => {
                      (e.currentTarget as HTMLTableRowElement).style.background = "var(--hover)";
                    }}
                    onMouseLeave={(e) => {
                      (e.currentTarget as HTMLTableRowElement).style.background = "transparent";
                    }}
                  >
                    <td style={{ padding: "6px 8px", color: "var(--text)", fontWeight: 500, maxWidth: "240px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {pkg.name}
                    </td>
                    <td style={{ padding: "6px 8px", color: "var(--muted)", maxWidth: "200px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {pkg.version || "-"}
                    </td>
                    <td style={{ padding: "6px 8px" }}>
                      <StatusBadge status={pkg.status} />
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

function StatusBadge({ status }: { status: string }) {
  const lower = status.toLowerCase();
  const isInstalled = lower === "installed";
  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 8px",
        borderRadius: "4px",
        fontSize: "10px",
        fontWeight: 500,
        background: isInstalled ? "var(--ok-glow)" : "var(--surface)",
        color: isInstalled ? "var(--ok)" : "var(--muted)",
        whiteSpace: "nowrap",
      }}
    >
      {status || "unknown"}
    </span>
  );
}

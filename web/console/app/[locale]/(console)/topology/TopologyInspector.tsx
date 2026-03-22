"use client";

import { useState, useCallback, useMemo } from "react";
import type { TopologyConnection, RelationshipType } from "./topologyCanvasTypes";
import { RELATIONSHIP_TYPES } from "./topologyCanvasTypes";
import { useFastStatus } from "../../../contexts/StatusContext";
import { freshnessColor } from "./topologyUtils";
import { friendlyTypeLabel, friendlySourceLabel } from "../../../console/taxonomy";
import { buildDisplayConnections } from "./topologyRelationshipGraph";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------


function connectionTypeBadgeClass(relationship: string): string {
  const rel = relationship.toLowerCase();
  if (rel === "runs_on" || rel === "hosted_on")
    return "bg-[var(--ok-glow)] text-[var(--ok)]";
  if (rel === "depends_on")
    return "bg-[var(--warn-glow)] text-[var(--warn)]";
  if (rel === "provides_to")
    return "bg-[var(--accent-subtle)] text-[var(--accent-text)]";
  return "bg-[var(--surface)] text-[var(--muted)]";
}

function originBadgeClass(origin: string): string {
  if (origin === "user") return "bg-[var(--accent-subtle)] text-[var(--accent-text)]";
  if (origin === "accepted") return "bg-[var(--ok-glow)] text-[var(--ok)]";
  return "bg-[var(--surface)] text-[var(--muted)]";
}

function statusDotColor(status: string): string {
  const s = status.toLowerCase();
  if (s === "online") return "#22c55e";
  if (s === "degraded") return "#eab308";
  if (s === "offline") return "#ef4444";
  return "var(--muted)";
}

// ---------------------------------------------------------------------------
// Row component
// ---------------------------------------------------------------------------

function InfoRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex justify-between py-1.5 text-xs border-b border-[var(--line)]">
      <span className="text-[var(--muted)]">{label}</span>
      <span className="text-[var(--text)] text-right max-w-[55%] truncate">{children}</span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface TopologyInspectorProps {
  mode: "asset" | "connection";
  // Asset mode
  assetID?: string | null;
  // Connection mode
  connectionID?: string | null;
  connections: TopologyConnection[];
  // Callbacks
  onUpdateConnection?: (id: string, updates: { relationship?: string; label?: string }) => void;
  onDeleteConnection?: (id: string) => void;
  onClose?: () => void;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function TopologyInspector({
  mode,
  assetID,
  connectionID,
  connections,
  onUpdateConnection,
  onDeleteConnection,
  onClose,
}: TopologyInspectorProps) {
  const fastStatus = useFastStatus();
  const assets = fastStatus?.assets;
  const assetByID = useMemo(() => new Map((assets ?? []).map((a) => [a.id, a])), [assets]);

  // --- Connection mode state ---
  const [updatingConn, setUpdatingConn] = useState(false);
  const [deletingConn, setDeletingConn] = useState(false);
  const [connActionError, setConnActionError] = useState<string | null>(null);

  // --- Derived data ---

  const selectedAsset = assetID ? assetByID.get(assetID) : undefined;

  const selectedConnection = connectionID
    ? connections.find((c) => c.id === connectionID)
    : undefined;

  const displayConnections = useMemo(
    () => buildDisplayConnections(assets ?? [], connections),
    [assets, connections],
  );

  const assetConnections = assetID
    ? displayConnections.filter(
        (c) => c.source_asset_id === assetID || c.target_asset_id === assetID,
      )
    : [];

  // Count children by type: assets where this asset is the source of
  // a containment-like connection (runs_on, hosted_on target points TO this asset)
  const childrenByType = (() => {
    if (!assetID) return new Map<string, number>();
    const counts = new Map<string, number>();
    for (const c of displayConnections) {
      // Children run_on / hosted_on this asset (target = this asset)
      const isChildConn =
        (c.relationship === "runs_on" || c.relationship === "hosted_on") &&
        c.target_asset_id === assetID;
      if (isChildConn) {
        const child = assetByID.get(c.source_asset_id);
        const t = child?.type ?? "unknown";
        counts.set(t, (counts.get(t) ?? 0) + 1);
      }
    }
    return counts;
  })();

  // --- Handlers ---

  const handleRelationshipChange = useCallback(
    async (value: string) => {
      if (!selectedConnection) return;
      setUpdatingConn(true);
      setConnActionError(null);
      try {
        await onUpdateConnection?.(selectedConnection.id, { relationship: value });
      } catch (err) {
        setConnActionError(err instanceof Error ? err.message : "Failed to update.");
      } finally {
        setUpdatingConn(false);
      }
    },
    [selectedConnection, onUpdateConnection],
  );

  const handleLabelChange = useCallback(
    async (value: string) => {
      if (!selectedConnection) return;
      try {
        await onUpdateConnection?.(selectedConnection.id, { label: value });
      } catch {
        // label edits are best-effort
      }
    },
    [selectedConnection, onUpdateConnection],
  );

  const handleDeleteConnection = useCallback(async () => {
    if (!selectedConnection) return;
    setDeletingConn(true);
    setConnActionError(null);
    try {
      await onDeleteConnection?.(selectedConnection.id);
    } catch (err) {
      setConnActionError(err instanceof Error ? err.message : "Failed to delete.");
      setDeletingConn(false);
    }
  }, [selectedConnection, onDeleteConnection]);

  // ---------------------------------------------------------------------------
  // Render: close button
  // ---------------------------------------------------------------------------

  const closeBtn = (
    <button
      type="button"
      onClick={onClose}
      aria-label="Close inspector"
      className="flex h-5 w-5 shrink-0 items-center justify-center rounded text-[var(--muted)] hover:bg-[var(--hover)] hover:text-[var(--text)] transition-colors duration-[var(--dur-fast)] text-sm leading-none"
    >
      &times;
    </button>
  );

  // ---------------------------------------------------------------------------
  // Render: connection mode
  // ---------------------------------------------------------------------------

  if (mode === "connection") {
    if (!selectedConnection) {
      return (
        <div className="flex h-full flex-col border-l border-[var(--panel-border)] bg-[var(--panel)] w-72">
          <div className="flex items-center justify-between border-b border-[var(--line)] px-4 py-3">
            <span className="text-xs font-medium text-[var(--muted)]">No connection selected</span>
            {closeBtn}
          </div>
        </div>
      );
    }

    const sourceAsset = assetByID.get(selectedConnection.source_asset_id);
    const targetAsset = assetByID.get(selectedConnection.target_asset_id);
    const sourceName = sourceAsset?.name ?? selectedConnection.source_asset_id;
    const targetName = targetAsset?.name ?? selectedConnection.target_asset_id;

    return (
      <div className="flex h-full flex-col border-l border-[var(--panel-border)] bg-[var(--panel)] w-72 overflow-y-auto">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-[var(--line)] px-4 py-3 gap-2">
          <p className="min-w-0 truncate text-xs font-medium text-[var(--text)]">
            {sourceName}
            <span className="mx-1 text-[var(--muted)]">→</span>
            {targetName}
          </p>
          {closeBtn}
        </div>

        {/* Body */}
        <div className="flex-1 px-4 py-3 space-y-1">
          {/* Type badge */}
          <div className="flex justify-between py-1.5 text-xs border-b border-[var(--line)]">
            <span className="text-[var(--muted)]">Type</span>
            <span
              className={`rounded-md px-1.5 py-0.5 text-[10px] ${connectionTypeBadgeClass(selectedConnection.relationship)}`}
            >
              {selectedConnection.relationship.replace(/_/g, " ")}
            </span>
          </div>

          {/* Origin badge */}
          <div className="flex justify-between py-1.5 text-xs border-b border-[var(--line)]">
            <span className="text-[var(--muted)]">Origin</span>
            <span
              className={`rounded-md px-1.5 py-0.5 text-[10px] ${originBadgeClass(selectedConnection.origin)}`}
            >
              {selectedConnection.origin}
            </span>
          </div>

          {/* Relationship type dropdown (editable) */}
          <div className="py-1.5 border-b border-[var(--line)] space-y-1">
            <p className="text-xs text-[var(--muted)]">Change type</p>
            <select
              value={selectedConnection.relationship}
              disabled={updatingConn || deletingConn}
              onChange={(e) => { void handleRelationshipChange(e.target.value); }}
              className="w-full rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-1 text-xs text-[var(--text)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)] disabled:opacity-50"
            >
              {RELATIONSHIP_TYPES.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {opt.label}
                </option>
              ))}
              {!RELATIONSHIP_TYPES.some((o) => o.value === selectedConnection.relationship) && (
                <option value={selectedConnection.relationship}>
                  {selectedConnection.relationship}
                </option>
              )}
            </select>
          </div>

          {/* Label (editable) */}
          <div className="py-1.5 border-b border-[var(--line)] space-y-1">
            <p className="text-xs text-[var(--muted)]">Label</p>
            <input
              type="text"
              defaultValue={selectedConnection.label ?? ""}
              placeholder="Optional label"
              disabled={updatingConn || deletingConn}
              onBlur={(e) => { void handleLabelChange(e.target.value); }}
              className="w-full rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-1 text-xs text-[var(--text)] placeholder:text-[var(--muted)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)] disabled:opacity-50"
            />
          </div>

          {/* Error */}
          {connActionError ? (
            <p className="text-xs text-[var(--bad)] pt-1">{connActionError}</p>
          ) : null}

          {/* Delete */}
          <div className="pt-3">
            <button
              type="button"
              disabled={deletingConn || updatingConn}
              onClick={() => { void handleDeleteConnection(); }}
              className="w-full h-7 rounded-md border border-[var(--bad)]/50 px-2.5 text-[10px] font-medium text-[var(--bad)] hover:bg-[var(--bad-glow)] transition-colors duration-[var(--dur-fast)] disabled:opacity-50"
            >
              {deletingConn ? "Removing…" : "Delete Connection"}
            </button>
          </div>
        </div>
      </div>
    );
  }

  // ---------------------------------------------------------------------------
  // Render: asset mode
  // ---------------------------------------------------------------------------

  if (!selectedAsset) {
    return (
      <div className="flex h-full flex-col border-l border-[var(--panel-border)] bg-[var(--panel)] w-72">
        <div className="flex items-center justify-between border-b border-[var(--line)] px-4 py-3">
          <span className="text-xs font-medium text-[var(--muted)]">No asset selected</span>
          {closeBtn}
        </div>
      </div>
    );
  }

  const freshness = freshnessColor(selectedAsset);
  const dotColor = statusDotColor(selectedAsset.status);
  const assetIP =
    selectedAsset.metadata?.ip_address ??
    selectedAsset.metadata?.host ??
    null;
  const assetUptime = selectedAsset.metadata?.uptime ?? null;

  return (
    <div className="flex h-full flex-col border-l border-[var(--panel-border)] bg-[var(--panel)] w-72 overflow-y-auto">
      {/* Header */}
      <div className="flex items-center gap-2 border-b border-[var(--line)] px-4 py-3">
        {/* Status dot */}
        <span
          className="shrink-0 h-2 w-2 rounded-full"
          style={{ backgroundColor: dotColor }}
          aria-label={selectedAsset.status}
        />
        <p className="min-w-0 flex-1 truncate text-xs font-medium text-[var(--text)]">
          {selectedAsset.name}
        </p>
        {closeBtn}
      </div>

      {/* Identity rows */}
      <div className="px-4 py-3">
        <InfoRow label="Type">
          {friendlyTypeLabel(selectedAsset.type)}
        </InfoRow>
        <InfoRow label="Source">
          {friendlySourceLabel(selectedAsset.source)}
        </InfoRow>
        {assetIP ? (
          <InfoRow label="IP">{assetIP}</InfoRow>
        ) : null}
        {assetUptime ? (
          <InfoRow label="Uptime">{assetUptime}</InfoRow>
        ) : null}
        <InfoRow label="Status">
          <span className={`rounded-md px-1.5 py-0.5 text-[10px] ${freshness.chip}`}>
            {freshness.label}
          </span>
        </InfoRow>
      </div>

      {/* Connections */}
      {assetConnections.length > 0 ? (
        <div className="px-4 pb-3">
          <p className="text-[11px] font-medium text-[var(--text)] mb-2">
            Connections ({assetConnections.length})
          </p>
          <ul className="space-y-1.5">
            {assetConnections.map((conn) => {
              const isSource = conn.source_asset_id === assetID;
              const peerID = isSource ? conn.target_asset_id : conn.source_asset_id;
              const peer = assetByID.get(peerID);
              const peerName = peer?.name ?? peerID;
              return (
                <li
                  key={conn.id}
                  className="flex items-center gap-1.5 rounded-md border border-[var(--line)]/70 bg-[var(--surface)] px-2 py-1.5"
                >
                  <span
                    className={`shrink-0 rounded-md px-1.5 py-0.5 text-[10px] ${connectionTypeBadgeClass(conn.relationship)}`}
                  >
                    {conn.relationship.replace(/_/g, " ")}
                  </span>
                  <span className="min-w-0 truncate text-xs text-[var(--text)]">
                    {isSource ? "→" : "←"} {peerName}
                  </span>
                  {"inferred" in conn && conn.inferred ? (
                    <span className="shrink-0 rounded bg-[var(--surface)] px-1 py-0.5 text-[10px] text-[var(--muted)]">
                      inferred
                    </span>
                  ) : null}
                </li>
              );
            })}
          </ul>
        </div>
      ) : (
        <div className="px-4 pb-3">
          <p className="text-[11px] font-medium text-[var(--text)] mb-1">Connections</p>
          <p className="text-xs text-[var(--muted)]">No connections</p>
        </div>
      )}

      {/* Children by type */}
      {childrenByType.size > 0 ? (
        <div className="px-4 pb-3 border-t border-[var(--line)] pt-3">
          <p className="text-[11px] font-medium text-[var(--text)] mb-2">Children</p>
          <ul className="space-y-1">
            {Array.from(childrenByType.entries()).map(([type, count]) => (
              <li key={type} className="flex justify-between text-xs">
                <span className="text-[var(--muted)]">{friendlyTypeLabel(type)}</span>
                <span className="text-[var(--text)]">{count}</span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

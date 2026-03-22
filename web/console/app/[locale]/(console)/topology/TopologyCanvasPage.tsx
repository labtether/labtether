"use client";

import { useState, useCallback, useEffect, useRef } from "react";
import { useTopologyData } from "./useTopologyData";
import { useTopologyUndo } from "./useTopologyUndo";
import TopologyCanvas from "./TopologyCanvas";
import TopologyInbox from "./TopologyInbox";
import { TopologyInspector } from "./TopologyInspector";
import TopologyTreeView from "./TopologyTreeView";
import { ConnectToDialog } from "./ConnectToDialog";
import { useFastStatus } from "../../../contexts/StatusContext";
import { TopologySearch } from "./TopologySearch";

type ViewMode = "canvas" | "tree";
type PanelMode = "inbox" | "inspector" | null;

export default function TopologyCanvasPage() {
  const {
    topology, isLoading, error, refresh,
    createZone, updateZone, deleteZone, setMembers,
    createConnection, updateConnection, deleteConnection,
    saveViewport, dismissAsset, autoPlace, resetTopology,
  } = useTopologyData();

  const { push: pushUndo, undo, redo, canUndo, canRedo } = useTopologyUndo();
  const fastStatus = useFastStatus();

  const [viewMode, setViewMode] = useState<ViewMode>("canvas");
  const [panelMode, setPanelMode] = useState<PanelMode>(null);
  const [selectedAssetID, setSelectedAssetID] = useState<string | null>(null);
  const [selectedConnectionID, setSelectedConnectionID] = useState<string | null>(null);
  const [connectFromAssetID, setConnectFromAssetID] = useState<string | null>(null);
  const [searchOpen, setSearchOpen] = useState(false);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "z") {
        if (e.shiftKey && canRedo) {
          e.preventDefault();
          redo();
        } else if (!e.shiftKey && canUndo) {
          e.preventDefault();
          undo();
        }
        return;
      }
      if ((e.metaKey || e.ctrlKey) && e.key === "f") {
        e.preventDefault();
        setSearchOpen(true);
      }
      if (e.key === "Delete" || e.key === "Backspace") {
        const target = e.target as HTMLElement;
        if (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable) return;
        if (selectedConnectionID) {
          deleteConnection(selectedConnectionID);
          setSelectedConnectionID(null);
        } else if (selectedAssetID) {
          const member = topology?.members.find(m => m.asset_id === selectedAssetID);
          if (member) {
            const remaining = topology!.members.filter(m => m.zone_id === member.zone_id && m.asset_id !== selectedAssetID);
            setMembers(member.zone_id, remaining);
          }
          setSelectedAssetID(null);
        }
      }
      if (e.key === "Escape") {
        setSelectedAssetID(null);
        setSelectedConnectionID(null);
        setPanelMode(null);
        setSearchOpen(false);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [undo, redo, canUndo, canRedo, selectedConnectionID, selectedAssetID, topology, deleteConnection, setMembers]);

  const unsortedCount = topology?.unsorted?.length ?? 0;

  // Use a ref for panelMode inside callbacks to avoid recreating them on panel changes,
  // which would propagate to TopologyCanvas and trigger node rebuilds.
  const panelModeRef = useRef(panelMode);
  panelModeRef.current = panelMode;

  const handleAssetSelect = useCallback((assetID: string | null) => {
    setSelectedAssetID(assetID);
    setSelectedConnectionID(null);
    if (assetID) setPanelMode("inspector");
    else if (panelModeRef.current === "inspector") setPanelMode(null);
  }, []);

  const handleConnectionSelect = useCallback((connID: string | null) => {
    setSelectedConnectionID(connID);
    setSelectedAssetID(null);
    if (connID) setPanelMode("inspector");
    else if (panelModeRef.current === "inspector") setPanelMode(null);
  }, []);

  const toggleInbox = useCallback(() => {
    setPanelMode(prev => prev === "inbox" ? null : "inbox");
    setSelectedAssetID(null);
    setSelectedConnectionID(null);
  }, []);

  // Loading state
  if (isLoading && !topology) {
    return (
      <div className="fixed inset-0 z-20 overflow-hidden md:left-52">
        <div className="flex h-full w-full items-center justify-center bg-[var(--surface)]/60 backdrop-blur-sm">
          <p className="text-sm text-[var(--muted)]">Loading topology canvas...</p>
        </div>
      </div>
    );
  }

  // Error state
  if (error && !topology) {
    return (
      <div className="fixed inset-0 z-20 overflow-hidden md:left-52">
        <div className="flex h-full w-full flex-col items-center justify-center gap-3">
          <p className="text-sm text-[var(--bad)]">Failed to load topology</p>
          <p className="text-xs text-[var(--muted)]">{error}</p>
          <button
            onClick={refresh}
            className="rounded-md bg-[var(--hover)] px-3 py-1.5 text-xs text-[var(--text)] transition-colors hover:bg-[var(--surface)]"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="fixed inset-0 z-20 overflow-hidden md:left-52">
      {/* Toolbar */}
      <div className="absolute left-3 right-3 top-3 z-10">
        <div
          className="glass flex items-center gap-2 rounded-xl border border-[var(--panel-border)] bg-[var(--panel-glass)] px-3 py-2"
          style={{ backdropFilter: "blur(var(--blur-md))", WebkitBackdropFilter: "blur(var(--blur-md))", boxShadow: "var(--shadow-sm)" }}
        >
          {/* View toggle */}
          <div className="flex rounded-lg bg-[var(--surface)] p-0.5">
            <button
              onClick={() => setViewMode("canvas")}
              aria-label="Switch to canvas view"
              className={`rounded-md px-3 py-1 text-xs font-medium transition-colors duration-[var(--dur-fast)] ${
                viewMode === "canvas"
                  ? "bg-[rgba(255,0,128,0.1)] text-[var(--accent)]"
                  : "text-[var(--muted)] hover:text-[var(--text)]"
              }`}
            >
              Canvas
            </button>
            <button
              onClick={() => setViewMode("tree")}
              aria-label="Switch to tree view"
              className={`rounded-md px-3 py-1 text-xs font-medium transition-colors duration-[var(--dur-fast)] ${
                viewMode === "tree"
                  ? "bg-[rgba(255,0,128,0.1)] text-[var(--accent)]"
                  : "text-[var(--muted)] hover:text-[var(--text)]"
              }`}
            >
              Tree
            </button>
          </div>

          {/* Divider */}
          <div className="h-5 w-px bg-[var(--line)]" />

          {/* Zone button */}
          <button
            onClick={() => createZone({ label: "New Zone", color: "blue" })}
            aria-label="Create new zone"
            className="rounded-md bg-[rgba(255,0,128,0.08)] px-2.5 py-1 text-xs text-[var(--accent)] transition-colors duration-[var(--dur-fast)] hover:bg-[rgba(255,0,128,0.15)]"
          >
            + Zone
          </button>

          {/* Spacer */}
          <div className="flex-1" />

          {/* Inbox toggle */}
          <button
            onClick={toggleInbox}
            aria-label={unsortedCount > 0 ? `Toggle unsorted inbox (${unsortedCount})` : "Toggle unsorted inbox"}
            className={`relative rounded-md px-2.5 py-1 text-xs transition-colors duration-[var(--dur-fast)] ${
              panelMode === "inbox"
                ? "bg-[var(--accent)] text-[var(--accent-contrast)]"
                : "bg-[var(--hover)] text-[var(--text)] hover:bg-[var(--surface)]"
            }`}
          >
            Inbox
            {unsortedCount > 0 && (
              <span className="absolute -right-1.5 -top-1.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-[var(--accent)] px-1 text-[10px] font-bold text-white animate-pulse">
                {unsortedCount}
              </span>
            )}
          </button>
        </div>
      </div>

      {/* Content area */}
      <div className="h-full w-full pt-14">
        {viewMode === "canvas" ? (
          topology ? (
            <TopologyCanvas
              topology={topology}
              onViewportChange={saveViewport}
              onZoneLabelChange={(id, label) => updateZone(id, { label })}
              onZoneToggleCollapse={(id) => {
                const zone = topology.zones.find(z => z.id === id);
                if (zone) updateZone(id, { collapsed: !zone.collapsed });
              }}
              onZoneDelete={deleteZone}
              onZoneMove={(id, x, y) => updateZone(id, { position: { x, y } })}
              onAssetSelect={handleAssetSelect}
              onConnectionSelect={handleConnectionSelect}
              onCreateConnection={(conn) => createConnection(conn)}
              onConnectTo={(assetId) => setConnectFromAssetID(assetId)}
              onCreateZone={() => createZone({ label: "New Zone", color: "blue" })}
              onMoveToZone={(assetId, zoneId) => {
                const existingMembers = topology.members.filter(m => m.zone_id === zoneId);
                setMembers(zoneId, [
                  ...existingMembers,
                  { zone_id: zoneId, asset_id: assetId, position: { x: 20, y: 20 + existingMembers.length * 50 }, sort_order: existingMembers.length },
                ]);
              }}
              onRemoveFromZone={(assetId) => {
                const member = topology.members.find(m => m.asset_id === assetId);
                if (member) {
                  const remaining = topology.members.filter(m => m.zone_id === member.zone_id && m.asset_id !== assetId);
                  setMembers(member.zone_id, remaining);
                }
              }}
              onChangeConnectionType={(connId, type) => updateConnection(connId, { relationship: type })}
              onDeleteConnection={deleteConnection}
              onResetLayout={resetTopology}
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center text-sm text-[var(--muted)]">
              No topology data
            </div>
          )
        ) : (
          topology ? (
            <TopologyTreeView
              topology={topology}
              selectedAssetID={selectedAssetID}
              onAssetSelect={handleAssetSelect}
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center text-sm text-[var(--muted)]">
              No topology data
            </div>
          )
        )}
      </div>

      {/* Search overlay */}
      {searchOpen && (
        <div className="absolute left-1/2 top-16 z-30 -translate-x-1/2">
          <TopologySearch
            onSelectResult={(assetID) => {
              handleAssetSelect(assetID);
              setSearchOpen(false);
            }}
            onClose={() => setSearchOpen(false)}
          />
        </div>
      )}

      {/* Connect-To dialog */}
      {connectFromAssetID && topology && (() => {
        const sourceAsset = fastStatus?.assets.find((a) => a.id === connectFromAssetID);
        return (
          <ConnectToDialog
            sourceAssetID={connectFromAssetID}
            sourceAssetName={sourceAsset?.name ?? connectFromAssetID}
            sourceAssetType={sourceAsset?.type ?? ""}
            topology={topology}
            onConnect={(targetID, relationship) => {
              createConnection({ source_asset_id: connectFromAssetID, target_asset_id: targetID, relationship });
              setConnectFromAssetID(null);
            }}
            onClose={() => setConnectFromAssetID(null)}
          />
        );
      })()}

      {/* Side panel */}
      {panelMode && (
        <div className="absolute bottom-0 right-0 top-14 z-20 w-72">
          {panelMode === "inbox" && topology && (
            <TopologyInbox
              unsortedAssetIDs={topology.unsorted}
              onAcceptSuggestion={(assetID, zoneID) => {
                // Add asset to the suggested zone
                const existingMembers = topology.members.filter(m => m.zone_id === zoneID);
                setMembers(zoneID, [...existingMembers, { zone_id: zoneID, asset_id: assetID, position: { x: 20, y: 20 + existingMembers.length * 50 }, sort_order: existingMembers.length }]);
              }}
              onDismiss={dismissAsset}
              onAutoPlace={autoPlace}
              onClose={() => setPanelMode(null)}
            />
          )}
          {panelMode === "inspector" && topology && (
            <TopologyInspector
              mode={selectedConnectionID ? "connection" : "asset"}
              assetID={selectedAssetID}
              connectionID={selectedConnectionID}
              connections={topology.connections}
              onUpdateConnection={(id, updates) => updateConnection(id, updates)}
              onDeleteConnection={deleteConnection}
              onClose={() => { setPanelMode(null); setSelectedAssetID(null); setSelectedConnectionID(null); }}
            />
          )}
        </div>
      )}
    </div>
  );
}

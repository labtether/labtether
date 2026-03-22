"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ChevronUp,
  FolderOpen,
  Loader2,
  Server,
  FolderKey,
  HardDrive,
  Globe,
  Columns2,
} from "lucide-react";
import type { FileSource } from "./useFileTabsState";
import {
  unifiedListFiles,
  type UnifiedFileEntry,
} from "./fileOpsClient";
import { fileIconComponent } from "./filesPageUtils";
import { formatSize } from "./fileWorkspaceUtils";
import { parentPath } from "./fileWorkspaceUtils";
import { useFileConnections } from "./useFileConnections";
import { useFastStatus } from "../../../contexts/StatusContext";
import { useConnectedAgents } from "../../../hooks/useConnectedAgents";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface SplitViewProps {
  leftSource: FileSource;
  leftProtocol?: string;
  rightSource: FileSource | null;
  rightProtocol?: string;
  onTransfer: (
    sourceFiles: string[],
    sourceSource: FileSource,
    destSource: FileSource,
    destPath: string,
  ) => void;
  onSelectRightSource: () => void;
  onSetSplitTarget?: (connectionId: string, protocol: string) => void;
}

// ---------------------------------------------------------------------------
// Protocol badge metadata
// ---------------------------------------------------------------------------

type ProtocolBadgeMeta = {
  label: string;
  icon: typeof Server;
  className: string;
};

const PROTOCOL_BADGES: Record<string, ProtocolBadgeMeta> = {
  agent: {
    label: "Agent",
    icon: Server,
    className: "bg-green-500/10 text-green-400 border-green-500/20",
  },
  sftp: {
    label: "SFTP",
    icon: FolderKey,
    className: "bg-blue-500/10 text-blue-400 border-blue-500/20",
  },
  smb: {
    label: "SMB",
    icon: HardDrive,
    className: "bg-orange-500/10 text-orange-400 border-orange-500/20",
  },
  ftp: {
    label: "FTP",
    icon: Server,
    className: "bg-teal-500/10 text-teal-400 border-teal-500/20",
  },
  webdav: {
    label: "WebDAV",
    icon: Globe,
    className: "bg-cyan-500/10 text-cyan-400 border-cyan-500/20",
  },
};

function getProtocolBadge(source: FileSource, protocol?: string): ProtocolBadgeMeta {
  if (source.type === "agent") return PROTOCOL_BADGES.agent;
  return PROTOCOL_BADGES[protocol ?? ""] ?? PROTOCOL_BADGES.sftp;
}

function ProtocolBadge({ source, protocol }: { source: FileSource; protocol?: string }) {
  const badge = getProtocolBadge(source, protocol);
  const Icon = badge.icon;
  return (
    <span
      className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium border ${badge.className}`}
    >
      <Icon className="w-3 h-3" />
      {badge.label}
    </span>
  );
}

// ---------------------------------------------------------------------------
// SplitPane -- lightweight single-pane file browser
// ---------------------------------------------------------------------------

interface SplitPaneProps {
  source: FileSource;
  protocol?: string;
  side: "left" | "right";
  dragOverlay: boolean;
  onDrop: (files: string[], destPath: string) => void;
  onDragStart: (files: string[], source: FileSource) => void;
}

function SplitPane({ source, protocol, side, dragOverlay, onDrop, onDragStart }: SplitPaneProps) {
  const [currentPath, setCurrentPath] = useState("/");
  const [entries, setEntries] = useState<UnifiedFileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const fetchIdRef = useRef(0);

  // Fetch file listing
  const fetchEntries = useCallback(
    async (path: string) => {
      const id = ++fetchIdRef.current;
      setLoading(true);
      setError(null);
      setSelected(new Set());
      try {
        const result = await unifiedListFiles(source, path);
        if (id !== fetchIdRef.current) return; // stale response
        const resolvedPath = result.path ?? path;
        setCurrentPath(resolvedPath);
        const sorted = [...result.entries].sort((a, b) => {
          if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
          return a.name.localeCompare(b.name);
        });
        setEntries(sorted);
      } catch (err) {
        if (id !== fetchIdRef.current) return;
        setError(err instanceof Error ? err.message : "Failed to list files");
        setEntries([]);
      } finally {
        if (id === fetchIdRef.current) setLoading(false);
      }
    },
    [source],
  );

  // Initial load
  useEffect(() => {
    void fetchEntries(source.type === "agent" ? "~" : "/");
  }, [fetchEntries, source]);

  const navigateTo = useCallback(
    (path: string) => {
      void fetchEntries(path);
    },
    [fetchEntries],
  );

  const navigateUp = useCallback(() => {
    const parent = parentPath(currentPath);
    void fetchEntries(parent);
  }, [currentPath, fetchEntries]);

  const handleEntryClick = useCallback(
    (entry: UnifiedFileEntry) => {
      if (entry.is_dir) {
        navigateTo(entry.path);
      } else {
        // Toggle selection for files
        setSelected((prev) => {
          const next = new Set(prev);
          if (next.has(entry.path)) {
            next.delete(entry.path);
          } else {
            next.add(entry.path);
          }
          return next;
        });
      }
    },
    [navigateTo],
  );

  const handleEntryDoubleClick = useCallback(
    (entry: UnifiedFileEntry) => {
      if (entry.is_dir) {
        navigateTo(entry.path);
      }
    },
    [navigateTo],
  );

  // ---------- Drag: start dragging selected files from this pane ----------
  const handleDragStart = useCallback(
    (event: React.DragEvent, entry: UnifiedFileEntry) => {
      // If the dragged item is not in the selection, drag just this item
      let dragPaths: string[];
      if (selected.has(entry.path)) {
        dragPaths = Array.from(selected);
      } else {
        dragPaths = [entry.path];
      }

      event.dataTransfer.setData(
        "application/x-labtether-files",
        JSON.stringify({ paths: dragPaths, side }),
      );
      event.dataTransfer.effectAllowed = "copy";
      onDragStart(dragPaths, source);
    },
    [selected, side, source, onDragStart],
  );

  // ---------- Drop: accept files from the other pane ----------
  const handleDragOver = useCallback((event: React.DragEvent) => {
    if (event.dataTransfer.types.includes("application/x-labtether-files")) {
      event.preventDefault();
      event.dataTransfer.dropEffect = "copy";
    }
  }, []);

  const handleDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      try {
        const raw = event.dataTransfer.getData("application/x-labtether-files");
        if (!raw) return;
        const data = JSON.parse(raw) as { paths: string[]; side: string };
        // Only accept drops from the opposite pane
        if (data.side === side) return;
        onDrop(data.paths, currentPath);
      } catch {
        // Ignore malformed drag data
      }
    },
    [currentPath, onDrop, side],
  );

  return (
    <div
      className="flex flex-col flex-1 min-w-0 min-h-0 relative"
      onDragOver={handleDragOver}
      onDrop={handleDrop}
    >
      {/* Pane header */}
      <div className="flex items-center gap-2 px-3 py-2 border-b border-[var(--line)] bg-[var(--panel)] flex-shrink-0">
        <ProtocolBadge source={source} protocol={protocol} />
        <button
          className="p-1 rounded text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer bg-transparent border-none flex-shrink-0"
          onClick={navigateUp}
          title="Go up"
        >
          <ChevronUp className="w-3.5 h-3.5" />
        </button>
        <span className="text-xs text-[var(--muted)] truncate flex-1 min-w-0">
          {currentPath}
        </span>
      </div>

      {/* File list */}
      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="flex items-center justify-center py-12">
            <Loader2
              className="w-5 h-5 text-[var(--muted)]"
              style={{ animation: "spin 1s linear infinite" }}
            />
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center gap-2 py-12 px-4 text-center">
            <p className="text-xs text-red-400">{error}</p>
            <button
              className="text-xs text-[var(--accent)] hover:underline cursor-pointer bg-transparent border-none"
              onClick={() => void fetchEntries(currentPath)}
            >
              Retry
            </button>
          </div>
        ) : entries.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 py-12 px-4 text-center">
            <FolderOpen className="w-8 h-8 text-[var(--muted)]" strokeWidth={1.5} />
            <p className="text-xs text-[var(--muted)]">Empty folder</p>
          </div>
        ) : (
          <div className="flex flex-col">
            {entries.map((entry) => {
              const Icon = fileIconComponent(entry.name, entry.is_dir);
              const isSelected = selected.has(entry.path);

              return (
                <div
                  key={entry.path}
                  className={`group flex items-center gap-2 px-3 py-1.5 cursor-pointer transition-colors duration-[var(--dur-instant)] ${
                    isSelected
                      ? "bg-[rgba(var(--accent-rgb),0.08)]"
                      : "hover:bg-[var(--hover)]"
                  }`}
                  onClick={() => handleEntryClick(entry)}
                  onDoubleClick={() => handleEntryDoubleClick(entry)}
                  draggable
                  onDragStart={(e) => handleDragStart(e, entry)}
                >
                  <Icon
                    className={`w-4 h-4 flex-shrink-0 ${
                      entry.is_dir ? "text-[var(--accent)]" : "text-[var(--muted)]"
                    }`}
                    strokeWidth={1.5}
                  />
                  <span className="text-xs text-[var(--text)] truncate flex-1 min-w-0">
                    {entry.name}
                  </span>
                  <span className="text-[10px] text-[var(--muted)] tabular-nums flex-shrink-0">
                    {entry.is_dir ? "" : formatSize(entry.size)}
                  </span>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Drop overlay */}
      {dragOverlay && (
        <div className="absolute inset-0 z-10 flex items-center justify-center bg-[rgba(var(--accent-rgb),0.08)] border-2 border-dashed border-[var(--accent)] rounded pointer-events-none">
          <p className="text-sm font-medium text-[var(--accent)]">Drop here to copy</p>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Right pane placeholder
// ---------------------------------------------------------------------------

const PROTOCOL_DOT_COLOR: Record<string, string> = {
  agent: "bg-green-500",
  sftp: "bg-blue-500",
  smb: "bg-orange-500",
  ftp: "bg-teal-500",
  webdav: "bg-cyan-500",
};

function RightPanePicker({ onSelect }: { onSelect: (source: FileSource, protocol: string) => void }) {
  const status = useFastStatus();
  const { connectedAgentIds } = useConnectedAgents();
  const { connections, loading } = useFileConnections();

  const connectedAgents = useMemo(() => {
    const allAssets = status?.assets ?? [];
    return allAssets.filter((asset) => connectedAgentIds.has(asset.id));
  }, [status, connectedAgentIds]);

  return (
    <div className="flex-1 flex flex-col min-w-0 overflow-y-auto">
      <div className="px-3 py-2 border-b border-[var(--line)] bg-[var(--panel)] flex-shrink-0">
        <span className="text-xs font-medium text-[var(--muted)] uppercase tracking-wider">
          Pick right pane
        </span>
      </div>

      {/* Connected agents */}
      {connectedAgents.length > 0 && (
        <div className="px-3 pt-3 pb-1">
          <span className="text-[10px] font-medium text-[var(--muted)] uppercase tracking-wider">Agents</span>
        </div>
      )}
      {connectedAgents.map((agent) => (
        <button
          key={agent.id}
          className="flex items-center gap-2 px-3 py-1.5 w-full text-left hover:bg-[var(--hover)] transition-colors cursor-pointer bg-transparent border-none"
          onClick={() => onSelect({ type: "agent", assetId: agent.id }, "agent")}
        >
          <span className="w-2 h-2 rounded-full bg-green-500 flex-shrink-0" />
          <span className="text-xs text-[var(--text)] truncate">{agent.name}</span>
        </button>
      ))}

      {/* Saved connections */}
      {loading ? (
        <div className="px-3 py-4 text-xs text-[var(--muted)]">Loading...</div>
      ) : connections.length > 0 ? (
        <>
          <div className="px-3 pt-3 pb-1">
            <span className="text-[10px] font-medium text-[var(--muted)] uppercase tracking-wider">Connections</span>
          </div>
          {connections.map((conn) => (
            <button
              key={conn.id}
              className="flex items-center gap-2 px-3 py-1.5 w-full text-left hover:bg-[var(--hover)] transition-colors cursor-pointer bg-transparent border-none"
              onClick={() => onSelect({ type: "connection", connectionId: conn.id }, conn.protocol)}
            >
              <span className={`w-2 h-2 rounded-full flex-shrink-0 ${PROTOCOL_DOT_COLOR[conn.protocol] ?? "bg-[var(--muted)]"}`} />
              <span className="text-xs text-[var(--text)] truncate">{conn.name}</span>
              <span className="text-[10px] text-[var(--muted)] uppercase ml-auto flex-shrink-0">{conn.protocol}</span>
            </button>
          ))}
        </>
      ) : null}

      {/* Empty state */}
      {connectedAgents.length === 0 && connections.length === 0 && !loading && (
        <div className="flex-1 flex flex-col items-center justify-center gap-3 px-4">
          <Columns2 className="w-10 h-10 text-[var(--muted)]" strokeWidth={1.25} />
          <p className="text-xs text-[var(--muted)] text-center">No connections available</p>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// SplitView -- dual-pane layout
// ---------------------------------------------------------------------------

export function SplitView({
  leftSource,
  leftProtocol,
  rightSource,
  rightProtocol,
  onTransfer,
  onSelectRightSource,
  onSetSplitTarget,
}: SplitViewProps) {
  const [leftDragOver, setLeftDragOver] = useState(false);
  const [rightDragOver, setRightDragOver] = useState(false);
  const [dragOrigin, setDragOrigin] = useState<"left" | "right" | null>(null);

  // Track drag origin to show overlay on the opposite pane only
  const handleDragStartLeft = useCallback(() => {
    setDragOrigin("left");
    setRightDragOver(true);
  }, []);

  const handleDragStartRight = useCallback(() => {
    setDragOrigin("right");
    setLeftDragOver(true);
  }, []);

  // Clear drag state when drag ends globally
  useEffect(() => {
    const clearDrag = () => {
      setLeftDragOver(false);
      setRightDragOver(false);
      setDragOrigin(null);
    };
    window.addEventListener("dragend", clearDrag);
    return () => window.removeEventListener("dragend", clearDrag);
  }, []);

  // Transfer handlers for each pane
  const handleDropOnLeft = useCallback(
    (files: string[], destPath: string) => {
      setLeftDragOver(false);
      setDragOrigin(null);
      if (rightSource) {
        onTransfer(files, rightSource, leftSource, destPath);
      }
    },
    [leftSource, rightSource, onTransfer],
  );

  const handleDropOnRight = useCallback(
    (files: string[], destPath: string) => {
      setRightDragOver(false);
      setDragOrigin(null);
      if (rightSource) {
        onTransfer(files, leftSource, rightSource, destPath);
      }
    },
    [leftSource, rightSource, onTransfer],
  );

  const handlePickRightSource = useCallback(
    (source: FileSource, protocol: string) => {
      if (onSetSplitTarget && source.type === "connection") {
        onSetSplitTarget(source.connectionId, protocol);
      } else if (onSetSplitTarget && source.type === "agent") {
        onSetSplitTarget(source.assetId, "agent");
      } else {
        onSelectRightSource();
      }
    },
    [onSetSplitTarget, onSelectRightSource],
  );

  return (
    <div className="flex flex-1 min-h-0 border border-[var(--panel-border)] rounded-lg overflow-hidden bg-[var(--panel-glass)]">
      {/* Left pane */}
      <SplitPane
        source={leftSource}
        protocol={leftProtocol}
        side="left"
        dragOverlay={leftDragOver && dragOrigin === "right"}
        onDrop={handleDropOnLeft}
        onDragStart={handleDragStartLeft}
      />

      {/* Divider */}
      <div className="w-px bg-[var(--line)] flex-shrink-0" />

      {/* Right pane */}
      {rightSource ? (
        <SplitPane
          source={rightSource}
          protocol={rightProtocol}
          side="right"
          dragOverlay={rightDragOver && dragOrigin === "left"}
          onDrop={handleDropOnRight}
          onDragStart={handleDragStartRight}
        />
      ) : (
        <RightPanePicker onSelect={handlePickRightSource} />
      )}
    </div>
  );
}

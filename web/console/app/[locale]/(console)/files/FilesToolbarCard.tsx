"use client";

import { useMemo } from "react";
import type { Asset } from "../../../console/models";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { Select } from "../../../components/ui/Input";
import type { WorkspaceClipboardMode } from "./useFileWorkspaceState";
import type { FileSource } from "./useFileTabsState";
import {
  Eye,
  EyeOff,
  RefreshCw,
  Upload,
  FolderPlus,
  ArrowUp,
  ChevronRight,
  Folder,
  Clipboard,
  Trash2,
  Download,
  X,
  List,
  LayoutGrid,
  Plug,
} from "lucide-react";

export type FileViewMode = "list" | "grid";

type FilesToolbarCardProps = {
  /** When provided, the toolbar is in tab-driven mode and omits the device selector. */
  source?: FileSource | null;
  /** Connection label to display in the toolbar badge (e.g. "SFTP myserver"). */
  sourceLabel?: string;
  /** Protocol name for the connection badge (e.g. "sftp"). */
  sourceProtocol?: string;

  // Legacy device-selector props (used when source is not provided).
  assets?: Asset[];
  target?: string;
  connectedAgentIds?: Set<string>;
  targetHasAgent?: boolean;
  onTargetChange?: (value: string) => void;

  showHidden: boolean;
  clipboardCount: number;
  clipboardMode: WorkspaceClipboardMode | null;
  selectionCount: number;
  currentPath: string;
  rootPath: string;
  onShowHiddenChange: (checked: boolean) => void;
  onDeleteSelected: () => void;
  onDownloadSelected: () => void;
  onUpload: () => void;
  onNewFolder: () => void;
  onRefresh: () => void;
  onNavigateToPath: (path: string) => void;
  onNavigateUp: () => void;
  onClearSelection: () => void;
  viewMode: FileViewMode;
  onViewModeChange: (mode: FileViewMode) => void;
};

export function FilesToolbarCard({
  source,
  sourceLabel,
  sourceProtocol,
  assets,
  target: legacyTarget,
  showHidden,
  connectedAgentIds,
  targetHasAgent: legacyTargetHasAgent,
  clipboardCount,
  clipboardMode,
  selectionCount,
  currentPath,
  rootPath,
  onTargetChange,
  onShowHiddenChange,
  onDeleteSelected,
  onDownloadSelected,
  onUpload,
  onNewFolder,
  onRefresh,
  onNavigateToPath,
  onNavigateUp,
  onClearSelection,
  viewMode,
  onViewModeChange,
}: FilesToolbarCardProps) {
  // Determine whether we have an active source -- either tab-driven or legacy.
  const hasSource = source != null;
  const target = hasSource
    ? (source.type === "agent" ? source.assetId : source.connectionId)
    : (legacyTarget ?? "");
  const targetHasAgent = hasSource ? true : (legacyTargetHasAgent ?? false);
  const useLegacySelector = !hasSource;
  const normalizedRoot = useMemo(() => {
    const trimmed = rootPath.trim();
    if (trimmed === "" || trimmed === "~") return "/";
    if (trimmed.length > 1 && trimmed.endsWith("/")) {
      return trimmed.replace(/\/+$/, "");
    }
    return trimmed;
  }, [rootPath]);

  const breadcrumbItems = useMemo(() => {
    const current = currentPath.trim();
    if (!current || current === normalizedRoot) return [] as Array<{ label: string; path: string }>;

    const rootPrefix = normalizedRoot === "/" ? "/" : `${normalizedRoot}/`;
    if (!current.startsWith(rootPrefix)) {
      const fallbackSegments = current.split("/").filter(Boolean);
      return fallbackSegments.map((segment, idx) => ({
        label: segment,
        path: `/${fallbackSegments.slice(0, idx + 1).join("/")}`,
      }));
    }

    const relative = current.slice(normalizedRoot.length).replace(/^\/+/, "");
    if (!relative) return [] as Array<{ label: string; path: string }>;
    const segments = relative.split("/").filter(Boolean);
    return segments.map((segment, idx) => {
      const suffix = segments.slice(0, idx + 1).join("/");
      const path = normalizedRoot === "/" ? `/${suffix}` : `${normalizedRoot}/${suffix}`;
      return { label: segment, path };
    });
  }, [currentPath, normalizedRoot]);

  const showBreadcrumb = target && targetHasAgent;
  const canNavigateUp = currentPath !== normalizedRoot && currentPath !== "~";
  const hasSelection = selectionCount > 0;

  return (
    <Card className="flex flex-col gap-2">
      {/* Row 1: Source selector or badge + breadcrumb */}
      <div className="flex items-center gap-3 min-h-[36px]">
        <div className="flex items-center gap-2 flex-shrink-0">
          {useLegacySelector ? (
            <>
              <Select
                aria-label="Target Device"
                value={target}
                onChange={(event) => onTargetChange?.(event.target.value)}
              >
                <option value="">Select a device...</option>
                {(assets ?? []).map((asset) => (
                  <option key={asset.id} value={asset.id}>
                    {(connectedAgentIds ?? new Set()).has(asset.id) ? "\uD83D\uDFE2 " : "\uD83D\uDD34 "}{asset.name} ({asset.platform || asset.source})
                  </option>
                ))}
              </Select>
              {target && (
                <span className="flex items-center gap-1.5 flex-shrink-0">
                  <span
                    className={`w-2 h-2 rounded-full flex-shrink-0 ${targetHasAgent ? "bg-[var(--ok)]" : "bg-[var(--bad)]"}`}
                    style={targetHasAgent
                      ? { boxShadow: "0 0 4px var(--ok-glow), 0 0 12px var(--ok-glow)" }
                      : { boxShadow: "0 0 4px var(--bad-glow), 0 0 12px var(--bad-glow)" }
                    }
                  />
                  <span className="text-xs text-[var(--muted)]">
                    {targetHasAgent ? "Connected" : "Offline"}
                  </span>
                </span>
              )}
            </>
          ) : (
            source && (
              <span className="flex items-center gap-1.5 flex-shrink-0">
                {source.type === "connection" && sourceProtocol && (
                  <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-[rgba(var(--accent-rgb),0.08)] border border-[var(--line)] text-xs text-[var(--accent)]">
                    <Plug className="w-3 h-3" />
                    <span className="uppercase font-medium">{sourceProtocol}</span>
                  </span>
                )}
                {sourceLabel && (
                  <span className="text-xs text-[var(--muted)] truncate max-w-[200px]" title={sourceLabel}>
                    {sourceLabel}
                  </span>
                )}
              </span>
            )
          )}
        </div>

        {showBreadcrumb && (
          <>
            <div className="w-px h-5 bg-[var(--line)] flex-shrink-0" />
            <nav className="flex items-center gap-0.5 min-w-0 overflow-x-auto text-sm">
              <button
                className="flex items-center gap-1 px-1.5 py-1 rounded-md text-[var(--text)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-instant)] cursor-pointer bg-transparent border-none text-sm flex-shrink-0"
                onClick={() => onNavigateToPath(normalizedRoot)}
                title={normalizedRoot === "/" ? "/" : normalizedRoot}
              >
                <Folder className="w-3.5 h-3.5 text-[var(--accent)]" />
                <span>{normalizedRoot === "/" ? "/" : "~"}</span>
              </button>
              {breadcrumbItems.map((item, idx) => (
                <span key={item.path} className="flex items-center flex-shrink-0">
                  <ChevronRight className="w-3 h-3 text-[var(--muted)]" />
                  <button
                    className={`px-1.5 py-1 rounded-md hover:bg-[var(--hover)] transition-colors duration-[var(--dur-instant)] cursor-pointer bg-transparent border-none text-sm ${
                      idx === breadcrumbItems.length - 1
                        ? "text-[var(--text)] font-medium"
                        : "text-[var(--muted)]"
                    }`}
                    onClick={() => onNavigateToPath(item.path)}
                  >
                    {item.label}
                  </button>
                </span>
              ))}
              {canNavigateUp && (
                <button
                  className="ml-1 p-1 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-instant)] cursor-pointer bg-transparent border-none flex-shrink-0"
                  onClick={onNavigateUp}
                  title="Go up a folder"
                >
                  <ArrowUp className="w-3.5 h-3.5" />
                </button>
              )}
            </nav>
          </>
        )}
      </div>

      {/* Row 2: Actions or Selection toolbar */}
      {target && targetHasAgent && (
        <div className="flex items-center justify-between gap-2 border-t border-[var(--line)] pt-2">
          {hasSelection ? (
            <div className="flex items-center gap-2 w-full">
              <span className="text-xs font-medium text-[var(--accent)] tabular-nums flex-shrink-0">
                {selectionCount} selected
              </span>
              <div className="flex items-center gap-1">
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={onDownloadSelected}
                  title="Download selected"
                >
                  <Download className="w-3.5 h-3.5" />
                  <span>Download</span>
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  onClick={onDeleteSelected}
                  title="Delete selected"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                  <span>Delete</span>
                </Button>
              </div>
              <div className="flex-1" />
              <button
                className="p-1 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-instant)] cursor-pointer bg-transparent border-none"
                onClick={onClearSelection}
                title="Clear selection"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          ) : (
            <>
              <div className="flex items-center gap-1">
                <button
                  className="flex items-center gap-1.5 px-2 py-1 rounded-md text-xs text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-instant)] cursor-pointer bg-transparent border-none"
                  onClick={() => onShowHiddenChange(!showHidden)}
                  title={showHidden ? "Hide hidden files" : "Show hidden files"}
                >
                  {showHidden ? <Eye className="w-3.5 h-3.5" /> : <EyeOff className="w-3.5 h-3.5" />}
                  <span>Hidden</span>
                </button>
                {clipboardCount > 0 && (
                  <span className="flex items-center gap-1.5 px-2 py-1 text-xs text-[var(--muted)]">
                    <Clipboard className="w-3.5 h-3.5" />
                    <span>
                      {clipboardCount} {clipboardMode === "cut" ? "cut" : "copied"}
                    </span>
                  </span>
                )}
              </div>
              <div className="flex items-center gap-1">
                <div className="flex items-center rounded-md border border-[var(--line)] overflow-hidden">
                  <button
                    className={`p-1.5 transition-colors duration-[var(--dur-instant)] cursor-pointer bg-transparent border-none ${
                      viewMode === "list"
                        ? "text-[var(--accent)] bg-[rgba(var(--accent-rgb),0.08)]"
                        : "text-[var(--muted)] hover:text-[var(--text)]"
                    }`}
                    onClick={() => onViewModeChange("list")}
                    title="List view"
                  >
                    <List className="w-4 h-4" />
                  </button>
                  <div className="w-px h-4 bg-[var(--line)]" />
                  <button
                    className={`p-1.5 transition-colors duration-[var(--dur-instant)] cursor-pointer bg-transparent border-none ${
                      viewMode === "grid"
                        ? "text-[var(--accent)] bg-[rgba(var(--accent-rgb),0.08)]"
                        : "text-[var(--muted)] hover:text-[var(--text)]"
                    }`}
                    onClick={() => onViewModeChange("grid")}
                    title="Grid view"
                  >
                    <LayoutGrid className="w-4 h-4" />
                  </button>
                </div>
                <button
                  className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-instant)] cursor-pointer bg-transparent border-none"
                  onClick={onRefresh}
                  title="Refresh"
                >
                  <RefreshCw className="w-4 h-4" />
                </button>
                <button
                  className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-instant)] cursor-pointer bg-transparent border-none"
                  onClick={onUpload}
                  title="Upload file"
                >
                  <Upload className="w-4 h-4" />
                </button>
                <button
                  className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-instant)] cursor-pointer bg-transparent border-none"
                  onClick={onNewFolder}
                  title="New folder"
                >
                  <FolderPlus className="w-4 h-4" />
                </button>
              </div>
            </>
          )}
        </div>
      )}
    </Card>
  );
}

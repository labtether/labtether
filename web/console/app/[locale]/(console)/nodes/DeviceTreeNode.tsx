"use client";

import type { KeyboardEvent, MouseEvent } from "react";
import { useCallback, useState } from "react";
import { useRouter } from "next/navigation";
import { ChevronDown, ChevronRight, GripVertical } from "lucide-react";
import { Badge } from "../../../components/ui/Badge";
import { MiniBar } from "../../../components/ui/MiniBar";
import { formatAge } from "../../../console/formatters";
import { friendlySourceLabel, friendlyTypeLabel, sourceIcon, isInfraHost } from "../../../console/taxonomy";
import { createComposite } from "../../../hooks/useComposites";
import { createEdge } from "../../../hooks/useEdges";
import { GroupHeader } from "./GroupHeader";
import { DropZoneOverlay } from "./DropZoneOverlay";
import type { DeviceTreeItem } from "./useDeviceTree";
import type { DeviceCardData } from "./nodesPageUtils";

type ManageCallbacks = {
  onMoveAsset?: (assetID: string, groupID: string | null) => Promise<void>;
  onMoveGroup?: (groupID: string, parentGroupID: string | null) => Promise<void>;
  onRenameGroup?: (id: string, name: string) => Promise<void>;
  onDeleteGroup?: (id: string) => Promise<void>;
  onCreateChildGroup?: (parentGroupID: string) => void;
  /** Called after a successful edge/composite creation to trigger a data refetch. */
  onRefetchEdges?: () => void;
};

type DeviceTreeNodeProps = {
  item: DeviceTreeItem;
  onToggle: (id: string) => void;
  isManaging?: boolean;
} & ManageCallbacks;

function isInteractiveDescendant(
  target: EventTarget | null,
  container: HTMLElement,
): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const interactive = target.closest(
    "a, button, input, select, textarea, summary, [role='button'], [role='link']",
  );
  return Boolean(interactive && interactive !== container);
}

// ── Drag & drop helpers ──

const DRAG_TYPE_DEVICE = "application/x-labtether-device";
const DRAG_TYPE_GROUP = "application/x-labtether-group";

function DeviceRow({
  card,
  depth,
  isManaging,
  hasChildren,
  expanded,
  onToggleExpand,
  hasProposal,
  childSummary,
  facets,
  onDragStart,
  onDragOver,
  onDragLeave,
  onDrop,
  isDropTarget: isDropTargetProp,
  dropHalf,
  canNest,
  onOverlayDragOver,
  onOverlayDragLeave,
  onOverlayDrop,
}: {
  card: DeviceCardData;
  depth: number;
  isManaging?: boolean;
  /** Whether this device has edge-based children. */
  hasChildren?: boolean;
  /** Whether the children section is expanded. */
  expanded?: boolean;
  /** Toggle expand/collapse for this device's children. */
  onToggleExpand?: () => void;
  /** True when there are pending edge proposals involving this asset. */
  hasProposal?: boolean;
  /** Summary of children when collapsed (e.g., "2 VMs · 3 Containers"). */
  childSummary?: string;
  /** Composite facet metadata for source pills. */
  facets?: Array<{ asset_id: string; source: string; type: string }>;
  onDragStart?: (e: React.DragEvent) => void;
  onDragOver?: (e: React.DragEvent) => void;
  onDragLeave?: (e: React.DragEvent) => void;
  onDrop?: (e: React.DragEvent) => void;
  /** Whether a compatible drag is hovering over this row (overlay active). */
  isDropTarget?: boolean;
  /** Which half the cursor is in during a drag-over-device operation. */
  dropHalf?: 'top' | 'bottom' | null;
  /** Whether this device can accept nested children. */
  canNest?: boolean;
  /** DragOver handler forwarded to the overlay zones. */
  onOverlayDragOver?: (e: React.DragEvent) => void;
  /** DragLeave handler forwarded to the overlay zones. */
  onOverlayDragLeave?: (e: React.DragEvent) => void;
  /** Drop handler forwarded to the overlay zones. */
  onOverlayDrop?: (e: React.DragEvent) => void;
}) {
  const router = useRouter();
  const { asset, freshness, cpu, mem, disk } = card;
  const isOffline = freshness === "offline" || freshness === "unknown";
  const Icon = sourceIcon(asset.source);
  const typeLabel = friendlyTypeLabel(asset.resource_kind ?? asset.type);
  const detailHref = `/nodes/${asset.id}`;

  // isDropTargetProp drives the group/manage drop highlight; overlay is driven
  // separately by isDropTargetProp (passed from the parent DeviceTreeNode).
  const showGroupHighlight = isDropTargetProp && !onOverlayDragOver;

  const handleClick = (event: MouseEvent<HTMLDivElement>) => {
    if (isManaging) return; // Suppress navigation in manage mode
    if (isInteractiveDescendant(event.target, event.currentTarget)) return;
    if (event.metaKey || event.ctrlKey) {
      window.open(detailHref, "_blank", "noopener,noreferrer");
      return;
    }
    router.push(detailHref);
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (isManaging) return;
    if (isInteractiveDescendant(event.target, event.currentTarget)) return;
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      router.push(detailHref);
    }
  };

  const handleDragOver = useCallback(
    (e: React.DragEvent) => {
      onDragOver?.(e);
    },
    [onDragOver],
  );

  const handleDragLeave = useCallback(
    (e: React.DragEvent) => {
      onDragLeave?.(e);
    },
    [onDragLeave],
  );

  const handleDropLocal = useCallback(
    (e: React.DragEvent) => {
      onDrop?.(e);
    },
    [onDrop],
  );

  return (
    <div className="relative">
      <DropZoneOverlay
        isActive={Boolean(isDropTargetProp && onOverlayDragOver)}
        dropHalf={dropHalf ?? null}
        canNest={canNest}
        onDragOver={onOverlayDragOver ?? (() => undefined)}
        onDragLeave={onOverlayDragLeave ?? (() => undefined)}
        onDrop={onOverlayDrop ?? (() => undefined)}
      />
      <div
        role={isManaging ? undefined : "link"}
        tabIndex={isManaging ? -1 : 0}
        onClick={handleClick}
        onKeyDown={handleKeyDown}
        draggable={isManaging}
        onDragStart={isManaging ? onDragStart : undefined}
        onDragOver={isManaging ? handleDragOver : undefined}
        onDragLeave={isManaging ? handleDragLeave : undefined}
        onDrop={isManaging ? handleDropLocal : undefined}
        className={`group/row flex items-center gap-2 rounded-md px-2 py-1.5
          hover:bg-[var(--hover)] transition-colors
          ${isManaging ? "cursor-grab" : "cursor-pointer"}
          ${showGroupHighlight ? "ring-1 ring-[var(--accent)] bg-[var(--accent-subtle)]" : ""}
          focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--control-focus-ring)]`}
        style={{
          paddingLeft: `${depth * 24 + 8}px`,
          transitionDuration: "var(--dur-instant)",
        }}
      >
        {/* Drag handle */}
      {isManaging ? (
        <span className="shrink-0 text-[var(--muted)] cursor-grab opacity-50 hover:opacity-100 transition-opacity">
          <GripVertical size={12} />
        </span>
      ) : null}

      {/* Expand/collapse chevron for infra hosts with children */}
      {hasChildren ? (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onToggleExpand?.();
          }}
          className="shrink-0 cursor-pointer p-0.5 rounded hover:bg-[var(--hover)] transition-colors"
          aria-expanded={expanded}
          aria-label={expanded ? "Collapse children" : "Expand children"}
        >
          {expanded ? (
            <ChevronDown size={12} className="text-[var(--muted)]" />
          ) : (
            <ChevronRight size={12} className="text-[var(--muted)]" />
          )}
        </button>
      ) : null}

      {/* Source icon */}
      <span className="flex items-center justify-center h-6 w-6 rounded-md bg-[var(--accent-subtle)] shrink-0">
        <Icon size={12} className="text-[var(--accent-text)] group-hover/row:text-[var(--accent)] transition-colors" />
      </span>

      {/* Status dot */}
      <Badge status={freshness} size="sm" dot />

      {/* Proposal indicator */}
      {hasProposal ? (
        <span
          className="inline-block h-1.5 w-1.5 rounded-full bg-[var(--accent)] shrink-0"
          title="Has pending link proposals"
        />
      ) : null}

      {/* Device name */}
      <span className="text-sm font-medium text-[var(--text)] truncate min-w-0">
        {asset.name}
      </span>

      {/* Composite facet source pills */}
      {facets && facets.length > 0 ? (
        <span className="inline-flex items-center gap-1 shrink-0">
          {facets.map((f) => (
            <span
              key={f.asset_id}
              className="rounded border border-[var(--accent)]/30 bg-[var(--accent)]/8 px-1.5 py-0.5 text-[10px] text-[var(--accent-text)] shrink-0"
              title={`Facet: ${friendlySourceLabel(f.source)} (${friendlyTypeLabel(f.type)})`}
            >
              {friendlySourceLabel(f.source)}
            </span>
          ))}
        </span>
      ) : null}

      {/* Source badge */}
      <span className="hidden sm:inline rounded border border-[var(--line)] bg-[var(--surface)] px-1.5 py-0.5 text-[10px] text-[var(--muted)] shrink-0">
        {friendlySourceLabel(asset.source)}
      </span>

      {/* Type badge */}
      <span className="hidden md:inline rounded border border-[var(--line)] bg-[var(--surface)] px-1.5 py-0.5 text-[10px] text-[var(--muted)] shrink-0">
        {typeLabel}
      </span>

      {/* Spacer */}
      <span className="flex-1 min-w-0" />

      {/* Collapsed child summary badge */}
      {hasChildren && !expanded && childSummary ? (
        <span className="hidden sm:inline rounded bg-[var(--surface)] px-1.5 py-0.5 text-[10px] text-[var(--muted)] shrink-0">
          {childSummary}
        </span>
      ) : null}

      {/* Inline metrics */}
      {!isOffline && (cpu != null || mem != null || disk != null) ? (
        <span className="hidden md:inline-flex items-center gap-3 shrink-0">
          {cpu != null ? <MiniBar value={cpu} label={`CPU ${Math.round(cpu)}%`} /> : null}
          {mem != null ? <MiniBar value={mem} label={`RAM ${Math.round(mem)}%`} /> : null}
          {disk != null ? <MiniBar value={disk} label={`DSK ${Math.round(disk)}%`} /> : null}
        </span>
      ) : null}

      {/* Offline age */}
      {isOffline ? (
        <span className="text-[10px] text-[var(--bad)] font-mono tabular-nums shrink-0">
          {freshness === "offline" ? "Offline" : "Unknown"} &middot; {formatAge(asset.last_seen_at)}
        </span>
      ) : null}

      {/* Navigate chevron (hidden in manage mode) */}
      {!isManaging ? (
        <ChevronRight
          size={12}
          className="text-[var(--muted)] opacity-0 group-hover/row:opacity-100 transition-opacity shrink-0"
        />
      ) : null}
      </div>
    </div>
  );
}

export function DeviceTreeNode({
  item,
  onToggle,
  isManaging,
  onMoveAsset,
  onMoveGroup,
  onRenameGroup,
  onDeleteGroup,
  onCreateChildGroup,
  onRefetchEdges,
}: DeviceTreeNodeProps) {
  // ── Split drop zone state (device-on-device drag) ──

  const [dragOverAssetID, setDragOverAssetID] = useState<string | null>(null);
  const [dropHalf, setDropHalf] = useState<'top' | 'bottom' | null>(null);

  // ── Drag event handlers ──

  const handleDeviceDragStart = useCallback(
    (e: React.DragEvent, assetID: string) => {
      e.dataTransfer.setData(DRAG_TYPE_DEVICE, assetID);
      e.dataTransfer.effectAllowed = "move";
    },
    [],
  );

  const handleGroupDragStart = useCallback(
    (e: React.DragEvent, groupID: string) => {
      e.dataTransfer.setData(DRAG_TYPE_GROUP, groupID);
      e.dataTransfer.effectAllowed = "move";
    },
    [],
  );

  const handleGroupDrop = useCallback(
    (e: React.DragEvent, targetGroupID: string) => {
      e.preventDefault();
      e.stopPropagation();
      const deviceID = e.dataTransfer.getData(DRAG_TYPE_DEVICE);
      const groupID = e.dataTransfer.getData(DRAG_TYPE_GROUP);
      if (deviceID) {
        void onMoveAsset?.(deviceID, targetGroupID);
      } else if (groupID && groupID !== targetGroupID) {
        void onMoveGroup?.(groupID, targetGroupID);
      }
    },
    [onMoveAsset, onMoveGroup],
  );

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
  }, []);

  // ── Overlay drag handlers (device-on-device) ──

  const makeOverlayDragOver = useCallback(
    (targetAssetID: string) => (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      const hasDraggedDevice = e.dataTransfer.types.includes(DRAG_TYPE_DEVICE);
      if (!hasDraggedDevice) return;
      e.dataTransfer.dropEffect = "move";
      setDragOverAssetID(targetAssetID);
      // Determine top vs bottom half from cursor position
      const rect = (e.currentTarget as HTMLElement).closest('.relative')?.getBoundingClientRect();
      if (rect) {
        const midY = rect.top + rect.height / 2;
        setDropHalf(e.clientY < midY ? 'top' : 'bottom');
      }
    },
    [],
  );

  const handleOverlayDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    // Only clear if we're leaving the outer relative container
    const related = e.relatedTarget as HTMLElement | null;
    const container = (e.currentTarget as HTMLElement).closest('.relative');
    if (!container || !related || !container.contains(related)) {
      setDragOverAssetID(null);
      setDropHalf(null);
    }
  }, []);

  const makeOverlayDrop = useCallback(
    (targetAssetID: string, targetIsInfraHost: boolean) => (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      const draggedDeviceID = e.dataTransfer.getData(DRAG_TYPE_DEVICE);
      if (!draggedDeviceID || draggedDeviceID === targetAssetID) {
        setDragOverAssetID(null);
        setDropHalf(null);
        return;
      }
      // Determine which half the drop occurred on
      const rect = (e.currentTarget as HTMLElement).closest('.relative')?.getBoundingClientRect();
      const half = rect && e.clientY < rect.top + rect.height / 2 ? 'top' : 'bottom';
      setDragOverAssetID(null);
      setDropHalf(null);
      if (half === 'top') {
        // Merge: create a composite with target as primary and dragged as facet
        void createComposite(targetAssetID, [draggedDeviceID]).then(() => {
          onRefetchEdges?.();
        });
      } else if (half === 'bottom' && targetIsInfraHost) {
        // Nest: create a contains edge from target → dragged
        void createEdge({
          source_asset_id: targetAssetID,
          target_asset_id: draggedDeviceID,
          relationship_type: 'contains',
          direction: 'downstream',
          criticality: 'medium',
        }).then(() => {
          onRefetchEdges?.();
        });
      }
    },
    [onRefetchEdges],
  );

  if (item.type === "group") {
    const rawGroupID = item.group.id;

    return (
      <div>
        <GroupHeader
          group={item.group}
          counts={item.counts}
          depth={item.depth}
          expanded={item.expanded}
          onToggle={() => onToggle(item.id)}
          isManaging={isManaging}
          onRename={onRenameGroup}
          onDelete={onDeleteGroup}
          onCreateChild={onCreateChildGroup}
          onDragStart={(e) => handleGroupDragStart(e, rawGroupID)}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={(e) => handleGroupDrop(e, rawGroupID)}
        />
        {item.expanded && item.children.length > 0 ? (
          <div className="relative">
            {/* Vertical tree line */}
            <span
              className="absolute top-0 bottom-0 w-px bg-[var(--line)]"
              style={{ left: `${item.depth * 24 + 22}px` }}
            />
            {item.children.map((child) => (
              <DeviceTreeNode
                key={child.id}
                item={child}
                onToggle={onToggle}
                isManaging={isManaging}
                onMoveAsset={onMoveAsset}
                onMoveGroup={onMoveGroup}
                onRenameGroup={onRenameGroup}
                onDeleteGroup={onDeleteGroup}
                onCreateChildGroup={onCreateChildGroup}
                onRefetchEdges={onRefetchEdges}
              />
            ))}
          </div>
        ) : null}
      </div>
    );
  }

  // Device node
  const hasEdgeChildren = item.children.length > 0;
  const targetAssetID = item.card.asset.id;
  const targetIsInfraHost = isInfraHost(item.card.asset);
  // Overlay is shown when a device drag is hovering over this specific row
  const overlayActive = dragOverAssetID === targetAssetID;

  return (
    <div>
      <DeviceRow
        card={item.card}
        depth={item.depth}
        isManaging={isManaging}
        hasChildren={hasEdgeChildren}
        expanded={item.expanded}
        onToggleExpand={() => onToggle(item.id)}
        hasProposal={item.hasProposal}
        childSummary={item.childSummary}
        facets={item.facets}
        onDragStart={(e) => handleDeviceDragStart(e, targetAssetID)}
        onDragOver={(e) => {
          // When managing, show split overlay for device-on-device drags
          if (isManaging && e.dataTransfer.types.includes(DRAG_TYPE_DEVICE)) {
            setDragOverAssetID(targetAssetID);
            const rect = (e.currentTarget as HTMLElement).closest('.relative')?.getBoundingClientRect();
            if (rect) {
              setDropHalf(e.clientY < rect.top + rect.height / 2 ? 'top' : 'bottom');
            }
          }
          handleDragOver(e);
        }}
        onDragLeave={(e) => {
          const related = e.relatedTarget as HTMLElement | null;
          const container = (e.currentTarget as HTMLElement).closest('.relative');
          if (!container || !related || !container.contains(related)) {
            setDragOverAssetID(null);
            setDropHalf(null);
          }
          handleDragLeave(e);
        }}
        onDrop={(e) => {
          // If a split overlay is active, let the overlay handlers take care of it
          if (overlayActive) return;
          // Dropping on a device — treat it as moving to the device's group
          e.preventDefault();
          e.stopPropagation();
          const deviceID = e.dataTransfer.getData(DRAG_TYPE_DEVICE);
          const groupID = e.dataTransfer.getData(DRAG_TYPE_GROUP);
          const targetGroup = item.card.asset.group_id || null;
          if (deviceID) {
            void onMoveAsset?.(deviceID, targetGroup);
          } else if (groupID) {
            void onMoveGroup?.(groupID, targetGroup);
          }
        }}
        isDropTarget={overlayActive}
        dropHalf={dropHalf}
        canNest={targetIsInfraHost}
        onOverlayDragOver={isManaging ? makeOverlayDragOver(targetAssetID) : undefined}
        onOverlayDragLeave={isManaging ? handleOverlayDragLeave : undefined}
        onOverlayDrop={isManaging ? makeOverlayDrop(targetAssetID, targetIsInfraHost) : undefined}
      />
      {item.expanded && item.children.length > 0 ? (
        <div className="relative">
          <span
            className="absolute top-0 bottom-0 w-px bg-[var(--line)]"
            style={{ left: `${item.depth * 24 + 22}px` }}
          />
          {item.children.map((child) => (
            <DeviceTreeNode
              key={child.id}
              item={child}
              onToggle={onToggle}
              isManaging={isManaging}
              onMoveAsset={onMoveAsset}
              onMoveGroup={onMoveGroup}
              onRenameGroup={onRenameGroup}
              onDeleteGroup={onDeleteGroup}
              onCreateChildGroup={onCreateChildGroup}
              onRefetchEdges={onRefetchEdges}
            />
          ))}
        </div>
      ) : null}
    </div>
  );
}

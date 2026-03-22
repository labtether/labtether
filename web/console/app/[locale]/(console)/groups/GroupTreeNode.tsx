"use client";

import {
  type KeyboardEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import {
  ChevronRight,
  Folder,
  FolderOpen,
  FolderPlus,
  GripVertical,
  MapPin,
  MoveRight,
  Pencil,
  Trash2,
} from "lucide-react";
import { GroupParentSelect } from "../../../components/GroupParentSelect";
import type { Group } from "../../../console/models";
import type { GroupTreeItem } from "./useGroupTree";

// ── Drag type constant ──

const DRAG_TYPE_GROUP = "application/x-labtether-group";

// ── Props ──

type GroupTreeNodeProps = {
  item: GroupTreeItem;
  allGroups: Group[];
  onToggle: (id: string) => void;
  isManaging: boolean;
  onEdit: (group: Group) => void;
  onDelete: (group: Group) => void;
  onCreateChild: (parentGroupID: string) => void;
  onMoveGroup: (groupID: string, parentGroupID: string | null) => Promise<void>;
  onRenameGroup: (id: string, name: string) => Promise<void>;
};

// ── Helpers ──

/** Check if nodeId is a descendant of potentialAncestorId (to prevent drop cycles). */
function isDescendantOf(
  nodeId: string,
  potentialAncestorId: string,
  groups: Group[],
): boolean {
  const byId = new Map(groups.map((g) => [g.id, g]));
  let current = nodeId;
  const visited = new Set<string>();
  while (current) {
    if (visited.has(current)) return false;
    visited.add(current);
    const g = byId.get(current);
    if (!g?.parent_group_id) return false;
    if (g.parent_group_id === potentialAncestorId) return true;
    current = g.parent_group_id;
  }
  return false;
}

// ── Component ──

export function GroupTreeNode({
  item,
  allGroups,
  onToggle,
  isManaging,
  onEdit,
  onDelete,
  onCreateChild,
  onMoveGroup,
  onRenameGroup,
}: GroupTreeNodeProps) {
  const { group, depth, expanded, children, deviceCount } = item;
  const FolderIcon = expanded ? FolderOpen : Folder;
  const hasChildren = children.length > 0;

  // Rename state
  const [isRenaming, setIsRenaming] = useState(false);
  const [renameValue, setRenameValue] = useState(group.name);
  const renameInputRef = useRef<HTMLInputElement>(null);
  const renamingInFlight = useRef(false);

  // Move-to inline panel state
  const [showMovePanel, setShowMovePanel] = useState(false);
  const [moveTarget, setMoveTarget] = useState<string>(
    group.parent_group_id ?? "",
  );
  const [moveError, setMoveError] = useState("");
  const [moveLoading, setMoveLoading] = useState(false);

  // Drag drop
  const [isDropTarget, setIsDropTarget] = useState(false);

  useEffect(() => {
    if (isRenaming) {
      renameInputRef.current?.focus();
      renameInputRef.current?.select();
    }
  }, [isRenaming]);

  const handleStartRename = useCallback(() => {
    if (!isManaging) return;
    setRenameValue(group.name);
    setIsRenaming(true);
  }, [isManaging, group.name]);

  const handleFinishRename = useCallback(async () => {
    if (renamingInFlight.current) return;
    const trimmed = renameValue.trim();
    if (!trimmed || trimmed === group.name) {
      setIsRenaming(false);
      return;
    }
    renamingInFlight.current = true;
    try {
      await onRenameGroup(group.id, trimmed);
    } catch {
      setRenameValue(group.name);
    }
    setIsRenaming(false);
    renamingInFlight.current = false;
  }, [renameValue, group.id, group.name, onRenameGroup]);

  const handleRenameKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      if (e.key === "Enter") {
        e.preventDefault();
        void handleFinishRename();
      } else if (e.key === "Escape") {
        e.preventDefault();
        setRenameValue(group.name);
        setIsRenaming(false);
      }
    },
    [handleFinishRename, group.name],
  );

  const handleMoveSelect = useCallback(
    async (newParentID: string) => {
      setMoveLoading(true);
      setMoveError("");
      try {
        await onMoveGroup(group.id, newParentID || null);
        setShowMovePanel(false);
      } catch (err) {
        setMoveError(err instanceof Error ? err.message : "Failed to move group");
      } finally {
        setMoveLoading(false);
      }
    },
    [group.id, onMoveGroup],
  );

  // Drag handlers
  const handleDragStart = useCallback(
    (e: React.DragEvent) => {
      e.dataTransfer.setData(DRAG_TYPE_GROUP, group.id);
      e.dataTransfer.effectAllowed = "move";
    },
    [group.id],
  );

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    setIsDropTarget(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDropTarget(false);
  }, []);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      setIsDropTarget(false);
      const draggedGroupID = e.dataTransfer.getData(DRAG_TYPE_GROUP);
      if (!draggedGroupID || draggedGroupID === group.id) return;
      // Prevent dropping onto own descendant (would create cycle)
      if (isDescendantOf(group.id, draggedGroupID, allGroups)) return;
      void onMoveGroup(draggedGroupID, group.id).catch(() => {
        /* drag-drop errors validated by backend */
      });
    },
    [group.id, onMoveGroup, allGroups],
  );

  return (
    <div>
      {/* ── Row ── */}
      <div
        className={`group/gtn flex w-full items-center gap-2 rounded-md px-2 py-1.5
          hover:bg-[var(--hover)] transition-colors select-none
          ${isDropTarget ? "ring-1 ring-[var(--accent)] bg-[var(--accent-subtle)]" : ""}`}
        style={{
          paddingLeft: `${depth * 24 + 8}px`,
          transitionDuration: "var(--dur-instant)",
        }}
        draggable={isManaging}
        onDragStart={isManaging ? handleDragStart : undefined}
        onDragOver={isManaging ? handleDragOver : undefined}
        onDragLeave={isManaging ? handleDragLeave : undefined}
        onDrop={isManaging ? handleDrop : undefined}
      >
        {/* Drag handle */}
        {isManaging ? (
          <span className="shrink-0 text-[var(--muted)] cursor-grab opacity-50 hover:opacity-100 transition-opacity">
            <GripVertical size={12} />
          </span>
        ) : null}

        {/* Chevron toggle or spacer */}
        {hasChildren ? (
          <button
            type="button"
            onClick={() => onToggle(item.id)}
            className="shrink-0 cursor-pointer p-0.5 rounded hover:bg-[var(--hover)] transition-colors"
            aria-expanded={expanded}
            aria-label={`${group.name} group`}
          >
            <ChevronRight
              size={14}
              className={`text-[var(--muted)] transition-transform ${expanded ? "rotate-90" : ""}`}
              style={{ transitionDuration: "var(--dur-fast)" }}
            />
          </button>
        ) : (
          <span className="shrink-0 w-[22px]" />
        )}

        {/* Folder icon — clicking also toggles */}
        <FolderIcon
          size={15}
          className="shrink-0 text-[var(--accent-text)] opacity-80 cursor-pointer"
          onClick={() => onToggle(item.id)}
        />

        {/* Group name */}
        {isRenaming ? (
          <input
            ref={renameInputRef}
            type="text"
            value={renameValue}
            onChange={(e) => setRenameValue(e.target.value)}
            onBlur={() => void handleFinishRename()}
            onKeyDown={handleRenameKeyDown}
            className="text-xs font-semibold uppercase tracking-wider text-[var(--text)] bg-transparent
              border-b border-[var(--accent)] outline-none py-0 px-0.5 min-w-0"
            style={{ width: `${Math.max(renameValue.length, 4)}ch` }}
          />
        ) : (
          <span
            className={`text-xs font-semibold uppercase tracking-wider text-[var(--muted)] truncate
              ${isManaging ? "cursor-text hover:text-[var(--text)] hover:underline decoration-dotted underline-offset-2" : "cursor-pointer"}`}
            onClick={isManaging ? handleStartRename : () => onToggle(item.id)}
          >
            {group.name}
          </span>
        )}

        {/* Location badge */}
        {group.location ? (
          <span
            className="hidden sm:inline-flex items-center gap-1 rounded border border-[var(--line)]
            bg-[var(--surface)] px-1.5 py-0.5 text-[10px] text-[var(--muted)] shrink-0"
          >
            <MapPin size={9} className="shrink-0" />
            {group.location}
          </span>
        ) : null}

        {/* Spacer line */}
        <span className="h-px flex-1 bg-[var(--line)]" />

        {/* Device count badge */}
        <span className="text-[10px] text-[var(--muted)] font-mono tabular-nums shrink-0">
          {deviceCount} device{deviceCount !== 1 ? "s" : ""}
        </span>

        {/* Manage mode actions */}
        {isManaging ? (
          <span className="flex items-center gap-1 shrink-0">
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onCreateChild(group.id);
              }}
              className="inline-flex items-center justify-center h-5 w-5 rounded text-[var(--muted)]
                hover:text-[var(--accent)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
              style={{ transitionDuration: "var(--dur-instant)" }}
              title={`Create child group inside ${group.name}`}
            >
              <FolderPlus size={11} />
            </button>
            <button
              type="button"
              disabled={moveLoading}
              onClick={(e) => {
                e.stopPropagation();
                setMoveTarget(group.parent_group_id ?? "");
                setMoveError("");
                setShowMovePanel((prev) => !prev);
              }}
              className="inline-flex items-center justify-center h-5 w-5 rounded text-[var(--muted)]
                hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
              style={{ transitionDuration: "var(--dur-instant)" }}
              title={`Move ${group.name} to another group`}
            >
              <MoveRight size={11} />
            </button>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onDelete(group);
              }}
              className="inline-flex items-center justify-center h-5 w-5 rounded text-[var(--muted)]
                hover:text-[var(--bad)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
              style={{ transitionDuration: "var(--dur-instant)" }}
              title={`Delete ${group.name}`}
            >
              <Trash2 size={11} />
            </button>
          </span>
        ) : (
          /* Normal mode hover actions */
          <span className="flex items-center gap-1 shrink-0 opacity-0 group-hover/gtn:opacity-100 transition-opacity">
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onEdit(group);
              }}
              className="inline-flex items-center justify-center h-5 w-5 rounded text-[var(--muted)]
                hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
              style={{ transitionDuration: "var(--dur-instant)" }}
              title={`Edit ${group.name}`}
            >
              <Pencil size={11} />
            </button>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onDelete(group);
              }}
              className="inline-flex items-center justify-center h-5 w-5 rounded text-[var(--muted)]
                hover:text-[var(--bad)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
              style={{ transitionDuration: "var(--dur-instant)" }}
              title={`Delete ${group.name}`}
            >
              <Trash2 size={11} />
            </button>
          </span>
        )}
      </div>

      {/* ── Move-to inline panel ── */}
      {showMovePanel ? (
        <div
          className="mx-2 mb-1 rounded-md border border-[var(--line)] bg-[var(--surface)]/60 p-2.5 space-y-2"
          style={{ marginLeft: `${depth * 24 + 8}px` }}
        >
          <p className="text-[10px] text-[var(--muted)]">
            Move <strong className="text-[var(--text)]">{group.name}</strong> to:
          </p>
          <GroupParentSelect
            groups={allGroups}
            value={moveTarget}
            onChange={setMoveTarget}
            excludeGroupId={group.id}
            disabled={moveLoading}
            label="New parent group"
          />
          {moveError ? (
            <p className="text-[10px] text-[var(--bad)]">{moveError}</p>
          ) : null}
          <div className="flex justify-end gap-1.5">
            <button
              type="button"
              className="text-[10px] text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)]
                px-2 py-0.5 rounded transition-colors cursor-pointer"
              style={{ transitionDuration: "var(--dur-instant)" }}
              onClick={() => setShowMovePanel(false)}
              disabled={moveLoading}
            >
              Cancel
            </button>
            <button
              type="button"
              className="text-[10px] text-[var(--accent-text)] bg-[var(--accent-subtle)] hover:bg-[var(--accent)]
                hover:text-white px-2 py-0.5 rounded transition-colors cursor-pointer disabled:opacity-50"
              style={{ transitionDuration: "var(--dur-instant)" }}
              onClick={() => void handleMoveSelect(moveTarget)}
              disabled={moveLoading || moveTarget === (group.parent_group_id ?? "")}
            >
              {moveLoading ? "Moving..." : "Move"}
            </button>
          </div>
        </div>
      ) : null}

      {/* ── Children ── */}
      {expanded && hasChildren ? (
        <div className="relative">
          {/* Vertical tree line */}
          <span
            className="absolute top-0 bottom-0 w-px bg-[var(--line)]"
            style={{ left: `${depth * 24 + 22}px` }}
          />
          {children.map((child) => (
            <GroupTreeNode
              key={child.id}
              item={child}
              allGroups={allGroups}
              onToggle={onToggle}
              isManaging={isManaging}
              onEdit={onEdit}
              onDelete={onDelete}
              onCreateChild={onCreateChild}
              onMoveGroup={onMoveGroup}
              onRenameGroup={onRenameGroup}
            />
          ))}
        </div>
      ) : null}
    </div>
  );
}

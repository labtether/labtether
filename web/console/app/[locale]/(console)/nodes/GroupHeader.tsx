"use client";

import { type KeyboardEvent, useCallback, useEffect, useRef, useState } from "react";
import { ChevronRight, Folder, FolderOpen, FolderPlus, GripVertical, MapPin, Trash2 } from "lucide-react";
import type { Group } from "../../../console/models";

type GroupHeaderProps = {
  group: Group;
  counts: { online: number; stale: number; offline: number };
  depth: number;
  expanded: boolean;
  onToggle: () => void;
  isManaging?: boolean;
  onRename?: (id: string, name: string) => Promise<void>;
  onDelete?: (id: string) => Promise<void>;
  onCreateChild?: (parentGroupID: string) => void;
  onDragStart?: (e: React.DragEvent) => void;
  onDragOver?: (e: React.DragEvent) => void;
  onDragLeave?: (e: React.DragEvent) => void;
  onDrop?: (e: React.DragEvent) => void;
};

export function GroupHeader({
  group,
  counts,
  depth,
  expanded,
  onToggle,
  isManaging,
  onRename,
  onDelete,
  onCreateChild,
  onDragStart,
  onDragOver,
  onDragLeave,
  onDrop,
}: GroupHeaderProps) {
  const total = counts.online + counts.stale + counts.offline;
  const FolderIcon = expanded ? FolderOpen : Folder;
  const isSynthetic = group.id === "__ungrouped__";

  const statusParts: string[] = [];
  if (counts.online > 0) statusParts.push(`${counts.online} online`);
  if (counts.stale > 0) statusParts.push(`${counts.stale} stale`);
  if (counts.offline > 0) statusParts.push(`${counts.offline} offline`);
  const statusText = statusParts.length > 0 ? statusParts.join(", ") : "empty";

  const [isRenaming, setIsRenaming] = useState(false);
  const [renameValue, setRenameValue] = useState(group.name);
  const [isDropTarget, setIsDropTarget] = useState(false);
  const renameInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (isRenaming) {
      renameInputRef.current?.focus();
      renameInputRef.current?.select();
    }
  }, [isRenaming]);

  const handleStartRename = useCallback(() => {
    if (isSynthetic || !isManaging) return;
    setRenameValue(group.name);
    setIsRenaming(true);
  }, [isSynthetic, isManaging, group.name]);

  const handleFinishRename = useCallback(async () => {
    const trimmed = renameValue.trim();
    if (!trimmed || trimmed === group.name) {
      setIsRenaming(false);
      return;
    }
    try {
      await onRename?.(group.id, trimmed);
    } catch {
      // Revert on error
      setRenameValue(group.name);
    }
    setIsRenaming(false);
  }, [renameValue, group.id, group.name, onRename]);

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

  const handleDelete = useCallback(async () => {
    if (isSynthetic) return;
    const ok = window.confirm(`Delete "${group.name}"? Assets inside will become ungrouped.`);
    if (!ok) return;
    try {
      await onDelete?.(group.id);
    } catch {
      // Error already handled upstream
    }
  }, [isSynthetic, group.id, group.name, onDelete]);

  const handleDragOver = useCallback(
    (e: React.DragEvent) => {
      setIsDropTarget(true);
      onDragOver?.(e);
    },
    [onDragOver],
  );

  const handleDragLeave = useCallback(
    (e: React.DragEvent) => {
      setIsDropTarget(false);
      onDragLeave?.(e);
    },
    [onDragLeave],
  );

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      setIsDropTarget(false);
      onDrop?.(e);
    },
    [onDrop],
  );

  const draggable = isManaging && !isSynthetic;

  return (
    <div
      className={`group/gh flex w-full items-center gap-2 rounded-md px-2 py-1.5
        hover:bg-[var(--hover)] transition-colors select-none
        ${isDropTarget ? "ring-1 ring-[var(--accent)] bg-[var(--accent-subtle)]" : ""}`}
      style={{
        paddingLeft: `${depth * 24 + 8}px`,
        transitionDuration: "var(--dur-instant)",
      }}
      draggable={draggable}
      onDragStart={draggable ? onDragStart : undefined}
      onDragOver={isManaging && !isSynthetic ? handleDragOver : undefined}
      onDragLeave={isManaging && !isSynthetic ? handleDragLeave : undefined}
      onDrop={isManaging && !isSynthetic ? handleDrop : undefined}
    >
      {/* Drag handle */}
      {isManaging && !isSynthetic ? (
        <span className="shrink-0 text-[var(--muted)] cursor-grab opacity-50 hover:opacity-100 transition-opacity">
          <GripVertical size={12} />
        </span>
      ) : null}

      {/* Chevron — acts as toggle */}
      <button
        type="button"
        onClick={onToggle}
        className="shrink-0 cursor-pointer p-0.5 rounded hover:bg-[var(--hover)] transition-colors"
        aria-expanded={expanded}
        aria-label={`${group.name} group, ${total} devices, ${statusText}`}
      >
        <ChevronRight
          size={14}
          className={`text-[var(--muted)] transition-transform ${expanded ? "rotate-90" : ""}`}
          style={{ transitionDuration: "var(--dur-fast)" }}
        />
      </button>

      {/* Folder icon */}
      <FolderIcon
        size={15}
        className="shrink-0 text-[var(--accent-text)] opacity-80"
      />

      {/* Group name: editable in manage mode */}
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
            ${isManaging && !isSynthetic ? "cursor-text hover:text-[var(--text)] hover:underline decoration-dotted underline-offset-2" : "cursor-pointer"}`}
          onClick={isManaging && !isSynthetic ? handleStartRename : onToggle}
        >
          {group.name}
        </span>
      )}

      {/* Location badge */}
      {group.location ? (
        <span className="hidden sm:inline-flex items-center gap-1 rounded border border-[var(--line)]
          bg-[var(--surface)] px-1.5 py-0.5 text-[10px] text-[var(--muted)] shrink-0">
          <MapPin size={9} className="shrink-0" />
          {group.location}
        </span>
      ) : null}

      {/* Spacer line */}
      <span className="h-px flex-1 bg-[var(--line)]" />

      {/* Manage mode actions */}
      {isManaging && !isSynthetic ? (
        <span className="flex items-center gap-1 shrink-0">
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              onCreateChild?.(group.id);
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
            onClick={(e) => {
              e.stopPropagation();
              void handleDelete();
            }}
            className="inline-flex items-center justify-center h-5 w-5 rounded text-[var(--muted)]
              hover:text-[var(--bad)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
            style={{ transitionDuration: "var(--dur-instant)" }}
            title={`Delete ${group.name}`}
          >
            <Trash2 size={11} />
          </button>
        </span>
      ) : null}

      {/* Status dots */}
      <span className="flex items-center gap-2 shrink-0">
        {counts.online > 0 ? (
          <span className="inline-flex items-center gap-1 text-[10px] text-[var(--ok)] font-mono tabular-nums">
            <span className="inline-block h-1.5 w-1.5 rounded-full bg-[var(--ok)]" />
            {counts.online}
          </span>
        ) : null}
        {counts.stale > 0 ? (
          <span className="inline-flex items-center gap-1 text-[10px] text-[var(--warn)] font-mono tabular-nums">
            <span className="inline-block h-1.5 w-1.5 rounded-full bg-[var(--warn)]" />
            {counts.stale}
          </span>
        ) : null}
        {counts.offline > 0 ? (
          <span className="inline-flex items-center gap-1 text-[10px] text-[var(--bad)] font-mono tabular-nums">
            <span className="inline-block h-1.5 w-1.5 rounded-full bg-[var(--bad)]"
              style={{ animation: "status-glow 1.5s ease-in-out infinite" }}
            />
            {counts.offline}
          </span>
        ) : null}
        <span className="text-[10px] text-[var(--muted)] font-mono tabular-nums">
          {total} device{total !== 1 ? "s" : ""}
        </span>
      </span>
    </div>
  );
}

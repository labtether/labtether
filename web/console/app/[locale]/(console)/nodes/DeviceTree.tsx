"use client";

import { useCallback, useState } from "react";
import { ChevronsDownUp, ChevronsUpDown, Server } from "lucide-react";
import { EmptyState } from "../../../components/ui/EmptyState";
import { DeviceTreeNode } from "./DeviceTreeNode";
import type { DeviceTreeItem } from "./useDeviceTree";

const DRAG_TYPE_DEVICE = "application/x-labtether-device";
const DRAG_TYPE_GROUP = "application/x-labtether-group";

type DeviceTreeProps = {
  tree: DeviceTreeItem[];
  onToggle: (id: string) => void;
  onExpandAll: () => void;
  onCollapseAll: () => void;
  isManaging?: boolean;
  onMoveAsset?: (assetID: string, groupID: string | null) => Promise<void>;
  onMoveGroup?: (groupID: string, parentGroupID: string | null) => Promise<void>;
  onRenameGroup?: (id: string, name: string) => Promise<void>;
  onDeleteGroup?: (id: string) => Promise<void>;
  onCreateChildGroup?: (parentGroupID: string) => void;
};

export function DeviceTree({
  tree,
  onToggle,
  onExpandAll,
  onCollapseAll,
  isManaging,
  onMoveAsset,
  onMoveGroup,
  onRenameGroup,
  onDeleteGroup,
  onCreateChildGroup,
}: DeviceTreeProps) {
  const [rootDropActive, setRootDropActive] = useState(false);

  const handleRootDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    setRootDropActive(true);
  }, []);

  const handleRootDragLeave = useCallback(() => {
    setRootDropActive(false);
  }, []);

  const handleRootDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setRootDropActive(false);
      const deviceID = e.dataTransfer.getData(DRAG_TYPE_DEVICE);
      const groupID = e.dataTransfer.getData(DRAG_TYPE_GROUP);
      if (deviceID) {
        void onMoveAsset?.(deviceID, null);
      } else if (groupID) {
        void onMoveGroup?.(groupID, null);
      }
    },
    [onMoveAsset, onMoveGroup],
  );

  if (tree.length === 0) {
    return (
      <EmptyState
        icon={Server}
        title="No devices match your search"
        description="Try a different search term."
      />
    );
  }

  // Check if any group nodes exist (for showing expand/collapse controls)
  const hasGroups = tree.some((item) => item.type === "group");

  return (
    <div className="rounded-lg border border-[var(--panel-border)] bg-[var(--panel-glass)] overflow-hidden">
      {/* Toolbar */}
      {hasGroups ? (
        <div className="flex items-center justify-end gap-1 px-2 py-1.5 border-b border-[var(--line)]">
          <button
            type="button"
            onClick={onExpandAll}
            className="inline-flex items-center gap-1 rounded px-2 py-0.5 text-[10px] text-[var(--muted)]
              hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors"
            style={{ transitionDuration: "var(--dur-instant)" }}
            title="Expand all groups"
          >
            <ChevronsUpDown size={12} />
            Expand
          </button>
          <button
            type="button"
            onClick={onCollapseAll}
            className="inline-flex items-center gap-1 rounded px-2 py-0.5 text-[10px] text-[var(--muted)]
              hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors"
            style={{ transitionDuration: "var(--dur-instant)" }}
            title="Collapse all groups"
          >
            <ChevronsDownUp size={12} />
            Collapse
          </button>
        </div>
      ) : null}

      {/* Tree items */}
      <div className="py-1 stagger-children">
        {tree.map((item) => (
          <DeviceTreeNode
            key={item.id}
            item={item}
            onToggle={onToggle}
            isManaging={isManaging}
            onMoveAsset={onMoveAsset}
            onMoveGroup={onMoveGroup}
            onRenameGroup={onRenameGroup}
            onDeleteGroup={onDeleteGroup}
            onCreateChildGroup={onCreateChildGroup}
          />
        ))}
      </div>

      {/* Root drop zone: ungroup by dropping here */}
      {isManaging ? (
        <div
          className={`mx-2 mb-2 rounded-md border border-dashed py-3 text-center text-[10px] text-[var(--muted)] transition-colors
            ${rootDropActive ? "border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent-text)]" : "border-[var(--line)]"}`}
          style={{ transitionDuration: "var(--dur-instant)" }}
          onDragOver={handleRootDragOver}
          onDragLeave={handleRootDragLeave}
          onDrop={handleRootDrop}
        >
          Drop here to ungroup
        </div>
      ) : null}
    </div>
  );
}

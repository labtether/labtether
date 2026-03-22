"use client";

import { useMemo } from "react";
import { TriangleAlert } from "lucide-react";
import type { Group } from "../../../console/models";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { Input } from "../../../components/ui/Input";
import { GroupParentSelect } from "../../../components/GroupParentSelect";

const NESTING_DEPTH_WARN = 5;

/** Returns the 0-based depth of a group in the hierarchy. Root groups are depth 0. */
function computeGroupDepth(groups: Group[], groupId: string): number {
  const byId = new Map(groups.map((g) => [g.id, g]));
  let depth = 0;
  let current = byId.get(groupId);
  while (current?.parent_group_id) {
    depth++;
    current = byId.get(current.parent_group_id);
    if (depth > 20) break; // guard against cycles
  }
  return depth;
}

type GroupCreateModalProps = {
  open: boolean;
  creating: boolean;
  createName: string;
  createSlug: string;
  createError: string;
  parentGroupId: string;
  onParentGroupIdChange: (value: string) => void;
  groups: Group[];
  onClose: () => void;
  onCreateNameChange: (value: string) => void;
  onCreateSlugChange: (value: string) => void;
  onSubmit: () => void;
};

export function GroupCreateModal({
  open,
  creating,
  createName,
  createSlug,
  createError,
  parentGroupId,
  onParentGroupIdChange,
  groups,
  onClose,
  onCreateNameChange,
  onCreateSlugChange,
  onSubmit,
}: GroupCreateModalProps) {
  // Depth of the new group = parent's depth + 1. Parent at depth 0 means the
  // new group is at depth 1. Warn when the resulting depth would exceed the threshold.
  const nestingDepth = useMemo(() => {
    if (!parentGroupId) return 0;
    return computeGroupDepth(groups, parentGroupId) + 1;
  }, [groups, parentGroupId]);

  const showNestingWarning = nestingDepth >= NESTING_DEPTH_WARN;

  if (!open) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={onClose}>
      <div onClick={(event) => { event.stopPropagation(); }}>
        <Card className="w-[34rem] max-w-[92vw] space-y-4">
          <h3 className="text-sm font-medium text-[var(--text)]">Create Group</h3>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <label className="space-y-1">
              <span className="text-[10px] text-[var(--muted)]">Name</span>
              <Input value={createName} onChange={(event) => onCreateNameChange(event.target.value)} placeholder="Home Lab" disabled={creating} />
            </label>
            <label className="space-y-1">
              <span className="text-[10px] text-[var(--muted)]">Slug</span>
              <Input
                value={createSlug}
                onChange={(event) => onCreateSlugChange(event.target.value.toLowerCase().replace(/[^a-z0-9-]/g, "-"))}
                placeholder="home-lab"
                maxLength={64}
                disabled={creating}
              />
            </label>
          </div>
          <GroupParentSelect
            groups={groups}
            value={parentGroupId}
            onChange={onParentGroupIdChange}
            disabled={creating}
            label="Parent Group"
          />
          {showNestingWarning ? (
            <div className="flex items-start gap-2 rounded-md border border-yellow-400/40 bg-yellow-400/10 px-3 py-2 text-xs text-yellow-600 dark:text-yellow-400">
              <TriangleAlert size={13} className="mt-px shrink-0" />
              <span>Groups nested deeper than 5 levels may affect dashboard performance.</span>
            </div>
          ) : null}
          {createError ? <p className="text-xs text-[var(--bad)]">{createError}</p> : null}
          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onClick={onClose} disabled={creating}>Cancel</Button>
            <Button variant="primary" onClick={onSubmit} disabled={creating}>
              {creating ? "Creating..." : "Create Group"}
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

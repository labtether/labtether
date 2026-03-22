"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import {
  ChevronsDownUp,
  ChevronsUpDown,
  FolderTree,
  MapPin,
  Plus,
  Settings2,
  TriangleAlert,
} from "lucide-react";
import { JumpChainEditor } from "../../../components/JumpChainEditor";
import { PageHeader } from "../../../components/PageHeader";
import { Card } from "../../../components/ui/Card";
import { Button } from "../../../components/ui/Button";
import { Input } from "../../../components/ui/Input";
import { EmptyState } from "../../../components/ui/EmptyState";
import { GroupParentSelect } from "../../../components/GroupParentSelect";
import { useFastStatus, useSlowStatus, useStatusControls } from "../../../contexts/StatusContext";
import type { Group, HopConfig } from "../../../console/models";
import { GroupTreeNode } from "./GroupTreeNode";
import { useGroupTree } from "./useGroupTree";
import { GroupCreateModal } from "./GroupCreateModal";
import { GroupDeleteModal } from "./GroupDeleteModal";
import {
  parseGroupMutationError,
  useGroupMutationActions,
  type CreateGroupInput,
  type UpdateGroupInput,
} from "./useGroupMutationActions";

// ── Drag type (root drop zone) ──

const DRAG_TYPE_GROUP = "application/x-labtether-group";

// ── Nesting depth helpers ──

const NESTING_DEPTH_WARN = 5;

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

// ── API helpers ──

async function apiMoveGroup(
  groupID: string,
  parentGroupID: string | null,
): Promise<void> {
  const response = await fetch(
    `/api/groups/${encodeURIComponent(groupID)}/move`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ parent_group_id: parentGroupID ?? "" }),
    },
  );
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as
      | { error?: string }
      | null;
    throw new Error(
      payload?.error ?? `Failed to move group (${response.status})`,
    );
  }
}

async function apiRenameGroup(
  groupID: string,
  name: string,
): Promise<void> {
  const response = await fetch(
    `/api/groups/${encodeURIComponent(groupID)}`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    },
  );
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as
      | { error?: string }
      | null;
    throw new Error(
      payload?.error ?? `Failed to rename group (${response.status})`,
    );
  }
}

// ── Page component ──

export default function GroupsPage() {
  const t = useTranslations('groups');
  const tc = useTranslations('common');
  const status = useFastStatus();
  const slowStatus = useSlowStatus();
  const { fetchStatus } = useStatusControls();
  const groups = useMemo(() => slowStatus?.groups ?? [], [slowStatus?.groups]);
  const assets = status?.assets;

  // Count devices per group (direct only — tree node computes cumulative)
  const deviceCountByGroup = useMemo(() => {
    const counts = new Map<string, number>();
    for (const asset of assets ?? []) {
      const groupID = asset.group_id?.trim();
      if (!groupID) continue;
      counts.set(groupID, (counts.get(groupID) ?? 0) + 1);
    }
    return counts;
  }, [assets]);

  // ── Tree ──

  const { tree, toggleExpand, expandAll, collapseAll } = useGroupTree({
    groups,
    deviceCountByGroup,
  });

  // ── Manage mode ──

  const [isManaging, setIsManaging] = useState(false);

  // Root drop zone
  const [isRootDropTarget, setIsRootDropTarget] = useState(false);

  const handleRootDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    setIsRootDropTarget(true);
  }, []);

  const handleRootDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsRootDropTarget(false);
  }, []);

  const handleRootDrop = useCallback(
    async (e: React.DragEvent) => {
      e.preventDefault();
      setIsRootDropTarget(false);
      const groupID = e.dataTransfer.getData(DRAG_TYPE_GROUP);
      if (!groupID) return;
      try {
        await apiMoveGroup(groupID, null);
        await fetchStatus();
      } catch {
        /* drag-drop errors validated by backend */
      }
    },
    [fetchStatus],
  );

  // ── Create modal ──

  const [showCreate, setShowCreate] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createSlug, setCreateSlug] = useState("");
  const [createError, setCreateError] = useState("");
  const [createParentGroupId, setCreateParentGroupId] = useState("");
  const [creating, setCreating] = useState(false);

  const { createGroup, updateGroup, deleteGroup } = useGroupMutationActions();

  const resetCreateForm = useCallback(() => {
    setCreateName("");
    setCreateSlug("");
    setCreateError("");
    setCreateParentGroupId("");
    setCreating(false);
  }, []);

  const openCreateModal = useCallback((parentID = "") => {
    setCreateName("");
    setCreateSlug("");
    setCreateError("");
    setCreateParentGroupId(parentID);
    setCreating(false);
    setShowCreate(true);
  }, []);

  const closeCreateModal = useCallback(() => {
    if (creating) return;
    setShowCreate(false);
    resetCreateForm();
  }, [creating, resetCreateForm]);

  const handleCreate = useCallback(async () => {
    const name = createName.trim();
    const slug = createSlug.trim().toLowerCase();

    if (!name || !slug) {
      setCreateError(t('create.nameSlugRequired'));
      return;
    }

    setCreating(true);
    setCreateError("");
    try {
      const input: CreateGroupInput = { name, slug };
      if (createParentGroupId) input.parent_group_id = createParentGroupId;
      await createGroup(input);
      await fetchStatus();
      setShowCreate(false);
      resetCreateForm();
    } catch (error) {
      setCreateError(parseGroupMutationError(error, "Failed to create group."));
    } finally {
      setCreating(false);
    }
  }, [
    t,
    createName,
    createSlug,
    createParentGroupId,
    createGroup,
    fetchStatus,
    resetCreateForm,
  ]);

  // ── Edit modal ──

  const [editTarget, setEditTarget] = useState<Group | null>(null);
  const [editName, setEditName] = useState("");
  const [editParentGroupId, setEditParentGroupId] = useState("");
  const [editJumpChainHops, setEditJumpChainHops] = useState<HopConfig[]>([]);
  const [editError, setEditError] = useState("");
  const [savingEdit, setSavingEdit] = useState(false);

  // Depth the edited group would occupy after the parent change.
  // Own depth = parent's depth + 1; root = 0.
  const editNestingDepth = useMemo(() => {
    if (!editParentGroupId) return 0;
    return computeGroupDepth(groups, editParentGroupId) + 1;
  }, [groups, editParentGroupId]);

  const showEditNestingWarning = editNestingDepth >= NESTING_DEPTH_WARN;

  const openEditModal = useCallback((group: Group) => {
    setEditTarget(group);
    setEditName(group.name);
    setEditParentGroupId(group.parent_group_id ?? "");
    setEditJumpChainHops(group.jump_chain?.hops ?? []);
    setEditError("");
    setSavingEdit(false);
  }, []);

  const closeEditModal = useCallback(() => {
    if (savingEdit) return;
    setEditTarget(null);
    setEditError("");
  }, [savingEdit]);

  // Escape key closes edit modal
  useEffect(() => {
    if (!editTarget) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !savingEdit) {
        e.preventDefault();
        closeEditModal();
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [editTarget, savingEdit, closeEditModal]);

  const handleSaveEdit = useCallback(async () => {
    if (!editTarget) return;
    const name = editName.trim();
    if (!name) {
      setEditError(t('edit.nameRequired'));
      return;
    }

    setSavingEdit(true);
    setEditError("");
    try {
      const nameChanged = name !== editTarget.name;
      const parentChanged =
        (editParentGroupId || null) !== (editTarget.parent_group_id ?? null);

      // Build the jump_chain value: null when empty, object when hops exist
      const prevHops = editTarget.jump_chain?.hops ?? [];
      const jumpChainChanged =
        JSON.stringify(editJumpChainHops) !== JSON.stringify(prevHops);
      const jumpChainValue =
        editJumpChainHops.length > 0
          ? { hops: editJumpChainHops }
          : null;

      const patch: UpdateGroupInput = {};
      if (nameChanged) patch.name = name;
      if (jumpChainChanged) patch.jump_chain = jumpChainValue;

      if (Object.keys(patch).length > 0) {
        await updateGroup(editTarget.id, patch);
      }
      if (parentChanged) {
        await apiMoveGroup(editTarget.id, editParentGroupId || null);
      }
      await fetchStatus();
      setEditTarget(null);
      setEditError("");
    } catch (error) {
      setEditError(parseGroupMutationError(error, "Failed to update group."));
      try { await fetchStatus(); } catch { /* ignore refresh failure */ }
    } finally {
      setSavingEdit(false);
    }
  }, [t, editTarget, editName, editParentGroupId, editJumpChainHops, updateGroup, fetchStatus]);

  // ── Delete modal ──

  const [deleteTarget, setDeleteTarget] = useState<Group | null>(null);
  const [deleteConfirmInput, setDeleteConfirmInput] = useState("");
  const [deleteError, setDeleteError] = useState("");
  const [deleting, setDeleting] = useState(false);

  const openDeleteModal = useCallback((group: Group) => {
    setDeleteTarget(group);
    setDeleteConfirmInput("");
    setDeleteError("");
  }, []);

  const closeDeleteModal = useCallback(() => {
    if (deleting) return;
    setDeleteTarget(null);
    setDeleteConfirmInput("");
    setDeleteError("");
  }, [deleting]);

  const confirmDelete = useCallback(async () => {
    if (!deleteTarget) return;
    if (deleteConfirmInput.trim() !== deleteTarget.name) {
      setDeleteError(t('delete.confirmHint'));
      return;
    }

    setDeleting(true);
    setDeleteError("");
    try {
      await deleteGroup(deleteTarget.id);
      await fetchStatus();
      setDeleteTarget(null);
      setDeleteConfirmInput("");
      setDeleteError("");
    } catch (error) {
      setDeleteError(parseGroupMutationError(error, "Failed to delete group."));
    } finally {
      setDeleting(false);
    }
  }, [t, deleteConfirmInput, deleteGroup, deleteTarget, fetchStatus]);

  // ── Move / rename callbacks for tree nodes ──

  const handleMoveGroup = useCallback(
    async (groupID: string, parentGroupID: string | null) => {
      await apiMoveGroup(groupID, parentGroupID);
      await fetchStatus();
    },
    [fetchStatus],
  );

  const handleRenameGroup = useCallback(
    async (id: string, name: string) => {
      await apiRenameGroup(id, name);
      await fetchStatus();
    },
    [fetchStatus],
  );

  // ── Render ──

  return (
    <>
      <PageHeader
        title={t('title')}
        subtitle={t('subtitle')}
        action={(
          <div className="flex items-center gap-2">
            <Button variant="secondary" size="sm" onClick={() => setIsManaging((v) => !v)}>
              <Settings2 size={14} />
              {isManaging ? t('done') : t('manage')}
            </Button>
            <Button variant="primary" size="sm" onClick={() => openCreateModal()}>
              <Plus size={14} />
              {t('newGroup')}
            </Button>
          </div>
        )}
      />

      <Card variant="flush">
        {/* Toolbar */}
        {groups.length > 0 ? (
          <div className="flex items-center gap-1 px-3 py-2 border-b border-[var(--line)]">
            <button
              type="button"
              className="inline-flex items-center gap-1.5 px-2 py-1 rounded text-[10px] text-[var(--muted)]
                hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
              style={{ transitionDuration: "var(--dur-instant)" }}
              onClick={expandAll}
              title="Expand all groups"
            >
              <ChevronsUpDown size={11} />
              {t('expandAll')}
            </button>
            <button
              type="button"
              className="inline-flex items-center gap-1.5 px-2 py-1 rounded text-[10px] text-[var(--muted)]
                hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
              style={{ transitionDuration: "var(--dur-instant)" }}
              onClick={collapseAll}
              title="Collapse all groups"
            >
              <ChevronsDownUp size={11} />
              {t('collapseAll')}
            </button>
          </div>
        ) : null}

        {/* Tree or empty state */}
        {groups.length === 0 ? (
          <div className="p-4">
            <EmptyState
              icon={MapPin}
              title={t('empty.title')}
              description={t('empty.description')}
              action={(
                <Button size="sm" variant="secondary" onClick={() => openCreateModal()}>
                  <Plus size={14} />
                  {t('empty.action')}
                </Button>
              )}
            />
          </div>
        ) : (
          <div className="p-1">
            {tree.map((item) => (
              <GroupTreeNode
                key={item.id}
                item={item}
                allGroups={groups}
                onToggle={toggleExpand}
                isManaging={isManaging}
                onEdit={openEditModal}
                onDelete={openDeleteModal}
                onCreateChild={(parentID) => openCreateModal(parentID)}
                onMoveGroup={handleMoveGroup}
                onRenameGroup={handleRenameGroup}
              />
            ))}

            {/* Root drop zone (manage mode only) */}
            {isManaging ? (
              <div
                className={`mt-1 mx-1 rounded-md border border-dashed text-center py-2 text-[10px] transition-colors
                  ${isRootDropTarget
                    ? "border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent-text)]"
                    : "border-[var(--line)] text-[var(--muted)]"
                  }`}
                style={{ transitionDuration: "var(--dur-instant)" }}
                onDragOver={handleRootDragOver}
                onDragLeave={handleRootDragLeave}
                onDrop={(e) => { void handleRootDrop(e); }}
              >
                <FolderTree size={10} className="inline mr-1.5 opacity-60" />
                {t('dropToRoot')}
              </div>
            ) : null}
          </div>
        )}
      </Card>

      {/* Create modal */}
      <GroupCreateModal
        open={showCreate}
        creating={creating}
        createName={createName}
        createSlug={createSlug}
        createError={createError}
        parentGroupId={createParentGroupId}
        onParentGroupIdChange={setCreateParentGroupId}
        groups={groups}
        onClose={closeCreateModal}
        onCreateNameChange={setCreateName}
        onCreateSlugChange={setCreateSlug}
        onSubmit={() => { void handleCreate(); }}
      />

      {/* Delete modal */}
      <GroupDeleteModal
        target={deleteTarget}
        deleteConfirmInput={deleteConfirmInput}
        deleteError={deleteError}
        deleting={deleting}
        onDeleteConfirmInputChange={setDeleteConfirmInput}
        onClose={closeDeleteModal}
        onConfirmDelete={() => { void confirmDelete(); }}
      />

      {/* Edit modal */}
      {editTarget ? (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
          onClick={closeEditModal}
        >
          <div onClick={(e) => e.stopPropagation()}>
            <Card className="w-[34rem] max-w-[92vw] space-y-4">
              <h3 className="text-sm font-medium text-[var(--text)]">{t('editModal.title')}</h3>
              <label className="block space-y-1">
                <span className="text-[10px] text-[var(--muted)]">{t('editModal.name')}</span>
                <Input
                  value={editName}
                  onChange={(e) => setEditName(e.target.value)}
                  disabled={savingEdit}
                />
              </label>
              <GroupParentSelect
                groups={groups}
                value={editParentGroupId}
                onChange={setEditParentGroupId}
                excludeGroupId={editTarget.id}
                disabled={savingEdit}
                label="Parent Group"
              />
              {showEditNestingWarning ? (
                <div className="flex items-start gap-2 rounded-md border border-yellow-400/40 bg-yellow-400/10 px-3 py-2 text-xs text-yellow-600 dark:text-yellow-400">
                  <TriangleAlert size={13} className="mt-px shrink-0" />
                  <span>Groups nested deeper than 5 levels may affect dashboard performance.</span>
                </div>
              ) : null}
              {/* Jump Chain (Network Access) */}
              <div className="space-y-1">
                <span className="text-[10px] text-[var(--muted)]">
                  {t('editModal.jumpChain')}
                </span>
                <JumpChainEditor
                  value={editJumpChainHops}
                  onChange={setEditJumpChainHops}
                  disabled={savingEdit}
                />
              </div>
              {editError ? (
                <p className="text-xs text-[var(--bad)]">{editError}</p>
              ) : null}
              <div className="flex items-center justify-end gap-2">
                <Button
                  variant="secondary"
                  onClick={closeEditModal}
                  disabled={savingEdit}
                >
                  {tc('cancel')}
                </Button>
                <Button
                  variant="primary"
                  onClick={() => { void handleSaveEdit(); }}
                  disabled={savingEdit}
                >
                  {savingEdit ? t('editModal.saving') : tc('save')}
                </Button>
              </div>
            </Card>
          </div>
        </div>
      ) : null}
    </>
  );
}

"use client";

import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { Input } from "../../../components/ui/Input";
import type { Group } from "../../../console/models";

type GroupDeleteModalProps = {
  target: Group | null;
  deleteConfirmInput: string;
  deleteError: string;
  deleting: boolean;
  onDeleteConfirmInputChange: (value: string) => void;
  onClose: () => void;
  onConfirmDelete: () => void;
};

export function GroupDeleteModal({
  target,
  deleteConfirmInput,
  deleteError,
  deleting,
  onDeleteConfirmInputChange,
  onClose,
  onConfirmDelete,
}: GroupDeleteModalProps) {
  if (!target) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={onClose}>
      <div onClick={(event) => { event.stopPropagation(); }}>
        <Card className="w-[32rem] max-w-[92vw] space-y-4">
          <h3 className="text-sm font-medium text-[var(--text)]">Delete Group</h3>
          <p className="text-xs text-[var(--muted)]">
            Delete <strong>{target.name}</strong>? This removes the group record.
            Devices currently assigned to this group may become unassigned.
          </p>
          <Input
            value={deleteConfirmInput}
            onChange={(event) => onDeleteConfirmInputChange(event.target.value)}
            placeholder={`Type "${target.name}" to confirm`}
            disabled={deleting}
          />
          {deleteError ? <p className="text-xs text-[var(--bad)]">{deleteError}</p> : null}
          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onClick={onClose} disabled={deleting}>Cancel</Button>
            <Button
              variant="danger"
              onClick={onConfirmDelete}
              disabled={deleting || deleteConfirmInput.trim() !== target.name}
            >
              {deleting ? "Deleting..." : "Delete Group"}
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

"use client";

import { type FormEvent, useState } from "react";
import { Modal } from "../../../components/ui/Modal";
import { Button } from "../../../components/ui/Button";
import { Input, Select } from "../../../components/ui/Input";
import type { Group } from "../../../console/models";

type GroupCreateDialogProps = {
  open: boolean;
  onClose: () => void;
  onSubmit: (name: string, parentGroupID?: string) => Promise<void>;
  groups: Group[];
};

export function GroupCreateDialog({ open, onClose, onSubmit, groups }: GroupCreateDialogProps) {
  const [name, setName] = useState("");
  const [parentGroupID, setParentGroupID] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;

    setLoading(true);
    setError(null);
    try {
      await onSubmit(trimmed, parentGroupID || undefined);
      setName("");
      setParentGroupID("");
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create group");
    } finally {
      setLoading(false);
    }
  };

  const handleClose = () => {
    if (loading) return;
    setName("");
    setParentGroupID("");
    setError(null);
    onClose();
  };

  // Only show real groups (not the synthetic __ungrouped__ group)
  const availableGroups = groups.filter((g) => g.id !== "__ungrouped__");

  return (
    <Modal open={open} onClose={handleClose} title="Create Group" className="max-w-sm">
      <form onSubmit={handleSubmit} className="px-5 py-4 space-y-4">
        {/* Name */}
        <div className="space-y-1.5">
          <label htmlFor="group-name" className="text-xs font-medium text-[var(--text-secondary)]">
            Name
          </label>
          <Input
            id="group-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Production, Lab Rack 2"
            autoFocus
            required
            error={!!error}
          />
        </div>

        {/* Parent group */}
        {availableGroups.length > 0 ? (
          <div className="space-y-1.5">
            <label htmlFor="group-parent" className="text-xs font-medium text-[var(--text-secondary)]">
              Parent Group
            </label>
            <Select
              id="group-parent"
              value={parentGroupID}
              onChange={(e) => setParentGroupID(e.target.value)}
            >
              <option value="">None (top-level)</option>
              {availableGroups.map((g) => (
                <option key={g.id} value={g.id}>
                  {g.name}
                </option>
              ))}
            </Select>
          </div>
        ) : null}

        {/* Error */}
        {error ? (
          <p className="text-xs text-[var(--bad)]">{error}</p>
        ) : null}

        {/* Actions */}
        <div className="flex justify-end gap-2 pt-1">
          <Button variant="ghost" size="sm" type="button" onClick={handleClose} disabled={loading}>
            Cancel
          </Button>
          <Button variant="primary" size="sm" type="submit" loading={loading} disabled={!name.trim()}>
            Create
          </Button>
        </div>
      </form>
    </Modal>
  );
}

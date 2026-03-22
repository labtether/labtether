"use client";

import { useState } from "react";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { Select } from "../../../components/ui/Input";
import type { HubUser } from "../../../hooks/useHubUsers";

type EditRoleDialogProps = {
  user: HubUser | null;
  onClose: () => void;
  onConfirm: (user: HubUser, role: string) => Promise<void>;
};

const roleOptions = [
  { value: "admin", label: "Admin" },
  { value: "operator", label: "Operator" },
  { value: "viewer", label: "Viewer" },
];

export function EditRoleDialog({ user, onClose, onConfirm }: EditRoleDialogProps) {
  const [role, setRole] = useState(user?.role === "owner" ? "admin" : (user?.role ?? "viewer"));
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  if (!user) return null;

  const handleConfirm = async () => {
    setSaving(true);
    setError("");
    try {
      await onConfirm(user, role);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to update role.");
      setSaving(false);
    }
  };

  const handleClose = () => {
    if (saving) return;
    setError("");
    onClose();
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={handleClose}
    >
      <div onClick={(e) => e.stopPropagation()}>
        <Card className="w-[28rem] max-w-[92vw] space-y-4">
          <h3 className="text-sm font-medium text-[var(--text)]">Edit Role</h3>
          <div className="space-y-1">
            <span className="text-[10px] text-[var(--muted)]">Username</span>
            <p className="text-sm text-[var(--text)] font-mono">{user.username}</p>
          </div>
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">Role</span>
            <Select
              value={role}
              onChange={(e) => setRole(e.target.value)}
              disabled={saving}
              className="w-full"
            >
              {roleOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </Select>
          </label>
          {error ? <p className="text-xs text-[var(--bad)]">{error}</p> : null}
          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onClick={handleClose} disabled={saving}>
              Cancel
            </Button>
            <Button
              variant="primary"
              loading={saving}
              onClick={() => { void handleConfirm(); }}
            >
              Update Role
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

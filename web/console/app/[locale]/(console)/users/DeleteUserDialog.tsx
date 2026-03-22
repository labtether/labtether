"use client";

import { useState } from "react";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { Input } from "../../../components/ui/Input";
import type { HubUser } from "../../../hooks/useHubUsers";

type DeleteUserDialogProps = {
  user: HubUser | null;
  onClose: () => void;
  onConfirm: (user: HubUser) => Promise<void>;
};

export function DeleteUserDialog({ user, onClose, onConfirm }: DeleteUserDialogProps) {
  const [confirmInput, setConfirmInput] = useState("");
  const [error, setError] = useState("");
  const [deleting, setDeleting] = useState(false);

  if (!user) return null;

  const handleConfirm = async () => {
    if (confirmInput.trim() !== user.username) {
      setError("Username does not match.");
      return;
    }
    setDeleting(true);
    setError("");
    try {
      await onConfirm(user);
      setConfirmInput("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete user.");
      setDeleting(false);
    }
  };

  const handleClose = () => {
    if (deleting) return;
    setConfirmInput("");
    setError("");
    onClose();
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={handleClose}
    >
      <div onClick={(e) => e.stopPropagation()}>
        <Card className="w-[32rem] max-w-[92vw] space-y-4">
          <h3 className="text-sm font-medium text-[var(--text)]">Delete User</h3>
          <p className="text-xs text-[var(--muted)]">
            This action cannot be undone. All sessions will be revoked.
          </p>
          <p className="text-xs text-[var(--muted)]">
            You are about to delete <strong className="text-[var(--text)]">{user.username}</strong>.
          </p>
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">Type the username to confirm</span>
            <Input
              value={confirmInput}
              onChange={(e) => setConfirmInput(e.target.value)}
              placeholder={user.username}
              disabled={deleting}
              autoFocus
            />
          </label>
          {error ? <p className="text-xs text-[var(--bad)]">{error}</p> : null}
          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onClick={handleClose} disabled={deleting}>
              Cancel
            </Button>
            <Button
              variant="danger"
              loading={deleting}
              disabled={deleting || confirmInput.trim() !== user.username}
              onClick={() => { void handleConfirm(); }}
            >
              Delete User
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

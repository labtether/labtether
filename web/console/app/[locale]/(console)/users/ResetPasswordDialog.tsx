"use client";

import { useState } from "react";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { Input } from "../../../components/ui/Input";
import type { HubUser } from "../../../hooks/useHubUsers";

type ResetPasswordDialogProps = {
  user: HubUser | null;
  onClose: () => void;
  onConfirm: (user: HubUser, password: string) => Promise<void>;
};

export function ResetPasswordDialog({ user, onClose, onConfirm }: ResetPasswordDialogProps) {
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  if (!user) return null;

  const handleConfirm = async () => {
    const pw = password.trim();
    const cpw = confirmPassword.trim();

    if (pw.length < 8) {
      setError("Password must be at least 8 characters.");
      return;
    }
    if (pw !== cpw) {
      setError("Passwords do not match.");
      return;
    }

    setSaving(true);
    setError("");
    try {
      await onConfirm(user, pw);
      setPassword("");
      setConfirmPassword("");
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to reset password.");
      setSaving(false);
    }
  };

  const handleClose = () => {
    if (saving) return;
    setPassword("");
    setConfirmPassword("");
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
          <h3 className="text-sm font-medium text-[var(--text)]">Reset Password</h3>
          <div className="space-y-1">
            <span className="text-[10px] text-[var(--muted)]">Username</span>
            <p className="text-sm text-[var(--text)] font-mono">{user.username}</p>
          </div>
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">New Password</span>
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="At least 8 characters"
              disabled={saving}
              autoFocus
            />
          </label>
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">Confirm Password</span>
            <Input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="Repeat password"
              disabled={saving}
            />
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
              Reset Password
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

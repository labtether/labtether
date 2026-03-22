"use client";

import { useState } from "react";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { Input, Select } from "../../../components/ui/Input";

type CreateUserDialogProps = {
  open: boolean;
  onClose: () => void;
  onConfirm: (payload: { username: string; password: string; role: string }) => Promise<void>;
};

const roleOptions = [
  { value: "admin", label: "Admin" },
  { value: "operator", label: "Operator" },
  { value: "viewer", label: "Viewer" },
];

const USERNAME_RE = /^[a-zA-Z0-9\-._]+$/;

export function CreateUserDialog({ open, onClose, onConfirm }: CreateUserDialogProps) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [role, setRole] = useState("viewer");
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);

  if (!open) return null;

  const handleConfirm = async () => {
    const u = username.trim().toLowerCase();
    const pw = password.trim();
    const cpw = confirmPassword.trim();

    if (!u) {
      setError("Username is required.");
      return;
    }
    if (!USERNAME_RE.test(u)) {
      setError("Username may only contain letters, numbers, hyphens, dots, and underscores.");
      return;
    }
    if (u.length > 32) {
      setError("Username must be 32 characters or fewer.");
      return;
    }
    if (pw.length < 8) {
      setError("Password must be at least 8 characters.");
      return;
    }
    if (pw !== cpw) {
      setError("Passwords do not match.");
      return;
    }

    setCreating(true);
    setError("");
    try {
      await onConfirm({ username: u, password: pw, role });
      setUsername("");
      setPassword("");
      setConfirmPassword("");
      setRole("viewer");
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create user.");
      setCreating(false);
    }
  };

  const handleClose = () => {
    if (creating) return;
    setUsername("");
    setPassword("");
    setConfirmPassword("");
    setRole("viewer");
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
          <h3 className="text-sm font-medium text-[var(--text)]">Add User</h3>
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">Username</span>
            <Input
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="ops-viewer"
              maxLength={32}
              disabled={creating}
              autoFocus
            />
          </label>
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">Password</span>
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="At least 8 characters"
              disabled={creating}
            />
          </label>
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">Confirm Password</span>
            <Input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="Repeat password"
              disabled={creating}
            />
          </label>
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">Role</span>
            <Select
              value={role}
              onChange={(e) => setRole(e.target.value)}
              disabled={creating}
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
            <Button variant="secondary" onClick={handleClose} disabled={creating}>
              Cancel
            </Button>
            <Button
              variant="primary"
              loading={creating}
              onClick={() => { void handleConfirm(); }}
            >
              Create User
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

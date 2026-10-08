"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import { safeJSON, extractError } from "../../../../lib/api";
import { TwoFactorSection } from "./TwoFactorSection";

type UserRole = "owner" | "admin" | "operator" | "viewer";

type UserRecord = {
  id: string;
  username: string;
  role: UserRole;
};

type UserAccessCardProps = {
  canManageUsers: boolean;
};

const roleOptions: Array<{ value: UserRole; label: string }> = [
  { value: "viewer", label: "Viewer (Read-only)" },
  { value: "operator", label: "Operator" },
  { value: "admin", label: "Admin" },
];

export function UserAccessCard({ canManageUsers }: UserAccessCardProps) {
  const [users, setUsers] = useState<UserRecord[]>([]);
  const [loadingUsers, setLoadingUsers] = useState(false);
  const [usersError, setUsersError] = useState<string | null>(null);
  const [statusMessage, setStatusMessage] = useState<string | null>(null);

  const [newUsername, setNewUsername] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [newRole, setNewRole] = useState<UserRole>("viewer");
  const [creatingUser, setCreatingUser] = useState(false);

  const [roleDraftByUserID, setRoleDraftByUserID] = useState<Record<string, UserRole>>({});
  const [passwordDraftByUserID, setPasswordDraftByUserID] = useState<Record<string, string>>({});
  const [savingUserID, setSavingUserID] = useState<string | null>(null);

  const hasUsers = users.length > 0;

  const resetStatus = () => {
    setUsersError(null);
    setStatusMessage(null);
  };

  const loadUsers = useCallback(async () => {
    if (!canManageUsers) {
      setUsers([]);
      return;
    }

    setLoadingUsers(true);
    setUsersError(null);

    try {
      const response = await fetch("/api/auth/users", { cache: "no-store" });
      const payload = await safeJSON(response);
      if (!response.ok) {
        setUsersError(extractError(payload, "Failed to load users"));
        return;
      }
      const nextUsers = parseUsers(payload);
      setUsers(nextUsers);
      setRoleDraftByUserID((current) => {
        const merged = { ...current };
        for (const user of nextUsers) {
          merged[user.id] = user.role;
        }
        return merged;
      });
    } catch {
      setUsersError("Users endpoint unavailable");
    } finally {
      setLoadingUsers(false);
    }
  }, [canManageUsers]);

  useEffect(() => {
    void loadUsers();
  }, [loadUsers]);

  const sortedUsers = useMemo(
    () => [...users].sort((left, right) => left.username.localeCompare(right.username)),
    [users],
  );

  const handleCreateUser = async () => {
    resetStatus();
    const username = newUsername.trim().toLowerCase();
		const password = newPassword;

    if (username === "") {
      setUsersError("Username is required");
      return;
    }
    if (password.length < 8) {
      setUsersError("Password must be at least 8 characters");
      return;
    }

    setCreatingUser(true);
    try {
      const response = await fetch("/api/auth/users", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, role: newRole }),
      });
      const payload = await safeJSON(response);
      if (!response.ok) {
        setUsersError(extractError(payload, "Failed to create user"));
        return;
      }

      const created = parseSingleUser(payload);
      if (created) {
        setUsers((current) => {
          const withoutExisting = current.filter((user) => user.id !== created.id);
          return [...withoutExisting, created];
        });
        setRoleDraftByUserID((current) => ({ ...current, [created.id]: created.role }));
      } else {
        await loadUsers();
      }

      setNewUsername("");
      setNewPassword("");
      setNewRole("viewer");
      setStatusMessage(`Created user ${username}`);
    } catch {
      setUsersError("Failed to create user");
    } finally {
      setCreatingUser(false);
    }
  };

  const handleSaveUser = async (user: UserRecord) => {
    resetStatus();

    const nextRole = roleDraftByUserID[user.id] ?? user.role;
		const nextPassword = passwordDraftByUserID[user.id] ?? "";

    const payload: Record<string, string> = {};
    if (nextRole !== user.role) {
      payload.role = nextRole;
    }
    if (nextPassword !== "") {
      payload.password = nextPassword;
    }

    if (Object.keys(payload).length === 0) {
      setStatusMessage(`No changes for ${user.username}`);
      return;
    }

    setSavingUserID(user.id);
    try {
      const response = await fetch(`/api/auth/users/${encodeURIComponent(user.id)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      const body = await safeJSON(response);
      if (!response.ok) {
        setUsersError(extractError(body, "Failed to update user"));
        return;
      }

      const updated = parseSingleUser(body);
      if (updated) {
        setUsers((current) => current.map((entry) => (entry.id === updated.id ? updated : entry)));
        setRoleDraftByUserID((current) => ({ ...current, [updated.id]: updated.role }));
      } else {
        await loadUsers();
      }
      setPasswordDraftByUserID((current) => ({ ...current, [user.id]: "" }));
      setStatusMessage(`Updated ${user.username}`);
    } catch {
      setUsersError("Failed to update user");
    } finally {
      setSavingUserID(null);
    }
  };

  if (!canManageUsers) {
    return (
      <Card className="mb-6">
        <h2>User Access</h2>
        <p className="text-sm text-[var(--muted)]">Only admin/owner users can manage accounts and roles.</p>
        <TwoFactorSection />
      </Card>
    );
  }

  return (
    <Card className="mb-6">
      <h2>User Access</h2>
      <p className="text-sm text-[var(--muted)]">
        Create local users, assign roles, and reset passwords. Viewer users are read-only.
      </p>

      <div className="mt-4 grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_220px_auto] md:items-end">
        <label className="text-xs text-[var(--muted)] flex flex-col gap-1.5">
          Username
          <Input value={newUsername} onChange={(event) => setNewUsername(event.target.value)} placeholder="ops-viewer" maxLength={64} />
        </label>
        <label className="text-xs text-[var(--muted)] flex flex-col gap-1.5">
          Temporary Password
          <Input
            type="password"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
            placeholder="At least 8 characters"
            maxLength={256}
          />
        </label>
        <label className="text-xs text-[var(--muted)] flex flex-col gap-1.5">
          Role
          <Select value={newRole} onChange={(event) => setNewRole(event.target.value as UserRole)}>
            {roleOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </Select>
        </label>
        <Button variant="primary" loading={creatingUser} onClick={() => void handleCreateUser()}>
          Create User
        </Button>
      </div>

      {usersError ? <p className="mt-3 text-sm text-[var(--bad)]">{usersError}</p> : null}
      {statusMessage ? <p className="mt-3 text-sm text-[var(--muted)]">{statusMessage}</p> : null}

      <div className="mt-4 border border-[var(--line)] rounded-xl overflow-hidden">
        <div className="grid grid-cols-[minmax(0,1fr)_200px_minmax(0,1fr)_auto] gap-3 px-3 py-2 bg-[var(--surface)] text-xs uppercase tracking-wide text-[var(--muted)]">
          <span>User</span>
          <span>Role</span>
          <span>Password Reset</span>
          <span />
        </div>

        {loadingUsers ? (
          <div className="px-3 py-3 text-sm text-[var(--muted)]">Loading users...</div>
        ) : null}

        {!loadingUsers && !hasUsers ? (
          <div className="px-3 py-3 text-sm text-[var(--muted)]">No users configured yet.</div>
        ) : null}

        {!loadingUsers
          ? sortedUsers.map((user) => {
              const isBuiltInAdmin = user.role === "owner";
              const isSaving = savingUserID === user.id;
              return (
                <div key={user.id} className="grid grid-cols-[minmax(0,1fr)_200px_minmax(0,1fr)_auto] gap-3 px-3 py-2 border-t border-[var(--line)] items-center">
                  <div className="min-w-0">
                    <p className="text-sm text-[var(--text)] truncate">{user.username}</p>
                    <p className="text-xs text-[var(--muted)] truncate">{user.id}</p>
                  </div>

                  <Select
                    value={roleDraftByUserID[user.id] ?? user.role}
                    onChange={(event) =>
                      setRoleDraftByUserID((current) => ({ ...current, [user.id]: event.target.value as UserRole }))
                    }
                    disabled={isBuiltInAdmin}
                  >
                    <option value="owner">Owner</option>
                    {roleOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </Select>

                  <Input
                    type="password"
                    value={passwordDraftByUserID[user.id] ?? ""}
                    onChange={(event) =>
                      setPasswordDraftByUserID((current) => ({ ...current, [user.id]: event.target.value }))
                    }
                    placeholder="Leave blank to keep current"
                    maxLength={256}
                  />

                  <Button variant="secondary" loading={isSaving} onClick={() => void handleSaveUser(user)}>
                    Save
                  </Button>
                </div>
              );
            })
          : null}
      </div>

      <TwoFactorSection />
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function parseUsers(payload: unknown): UserRecord[] {
  if (!payload || typeof payload !== "object") {
    return [];
  }
  const users = (payload as { users?: unknown }).users;
  if (!Array.isArray(users)) {
    return [];
  }
  return users
    .map((entry) => parseUser(entry))
    .filter((entry): entry is UserRecord => entry !== null);
}

function parseSingleUser(payload: unknown): UserRecord | null {
  if (!payload || typeof payload !== "object") {
    return null;
  }
  return parseUser((payload as { user?: unknown }).user);
}

function parseUser(payload: unknown): UserRecord | null {
  if (!payload || typeof payload !== "object") {
    return null;
  }
  const record = payload as Record<string, unknown>;
  if (typeof record.id !== "string" || typeof record.username !== "string") {
    return null;
  }
  const role = normalizeRole(record.role);
  return {
    id: record.id,
    username: record.username,
    role,
  };
}

function normalizeRole(role: unknown): UserRole {
  const value = typeof role === "string" ? role.trim().toLowerCase() : "";
  switch (value) {
    case "owner":
      return "owner";
    case "admin":
      return "admin";
    case "operator":
      return "operator";
    default:
      return "viewer";
  }
}

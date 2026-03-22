import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { useAuth } from "../../../../contexts/AuthContext";
import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import { safeJSON, extractError } from "../../../../lib/api";

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
    const password = newPassword.trim();

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
    const nextPassword = (passwordDraftByUserID[user.id] ?? "").trim();

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
// Two-Factor Authentication Section
// ---------------------------------------------------------------------------

type TwoFactorState = "idle" | "setting-up" | "verifying";

function TwoFactorSection() {
  const { user, refreshUser } = useAuth();
  const totpEnabled = user?.totp_enabled === true;

  const [tfState, setTfState] = useState<TwoFactorState>("idle");
  const [setupSecret, setSetupSecret] = useState("");
  const [setupURI, setSetupURI] = useState("");
  const [verifyCode, setVerifyCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [disableCode, setDisableCode] = useState("");
  const [regenCode, setRegenCode] = useState("");
  const [showDisable, setShowDisable] = useState(false);
  const [showRegen, setShowRegen] = useState(false);
  const [tfLoading, setTfLoading] = useState(false);
  const [tfError, setTfError] = useState<string | null>(null);
  const [tfMessage, setTfMessage] = useState<string | null>(null);
  const [secretCopied, setSecretCopied] = useState(false);
  const [codesCopied, setCodesCopied] = useState(false);

  const resetTfStatus = () => {
    setTfError(null);
    setTfMessage(null);
  };

  const resetTfState = () => {
    setTfState("idle");
    setSetupSecret("");
    setSetupURI("");
    setVerifyCode("");
    setRecoveryCodes([]);
    setDisableCode("");
    setRegenCode("");
    setShowDisable(false);
    setShowRegen(false);
    setSecretCopied(false);
    setCodesCopied(false);
    resetTfStatus();
  };

  const handleStartSetup = async () => {
    resetTfStatus();
    setTfLoading(true);
    try {
      const response = await fetch("/api/auth/2fa/setup", { method: "POST" });
      const payload = await safeJSON(response);
      if (!response.ok) {
        setTfError(extractError(payload, "Failed to start 2FA setup"));
        return;
      }
      const data = payload as { secret?: string; uri?: string } | null;
      setSetupSecret(data?.secret ?? "");
      setSetupURI(data?.uri ?? "");
      setTfState("setting-up");
    } catch {
      setTfError("2FA setup endpoint unavailable");
    } finally {
      setTfLoading(false);
    }
  };

  const handleVerify = async () => {
    resetTfStatus();
    const code = verifyCode.trim();
    if (code.length !== 6 || !/^\d{6}$/.test(code)) {
      setTfError("Enter a valid 6-digit code");
      return;
    }

    setTfLoading(true);
    try {
      const response = await fetch("/api/auth/2fa/verify", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ code }),
      });
      const payload = await safeJSON(response);
      if (!response.ok) {
        setTfError(extractError(payload, "Verification failed"));
        return;
      }
      const data = payload as { recovery_codes?: string[] } | null;
      setRecoveryCodes(data?.recovery_codes ?? []);
      setTfState("verifying");
      setTfMessage("Two-factor authentication enabled successfully");
      await refreshUser();
    } catch {
      setTfError("2FA verify endpoint unavailable");
    } finally {
      setTfLoading(false);
    }
  };

  const handleDisable = async () => {
    resetTfStatus();
    const code = disableCode.trim();
    const isValidTOTP = /^\d{6}$/.test(code);
    const isValidRecovery = /^[0-9a-f]{8}-[0-9a-f]{8}$/i.test(code);
    if (!isValidTOTP && !isValidRecovery) {
      setTfError("Enter a 6-digit code or recovery code");
      return;
    }

    setTfLoading(true);
    try {
      const response = await fetch("/api/auth/2fa", {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ code }),
      });
      const payload = await safeJSON(response);
      if (!response.ok) {
        setTfError(extractError(payload, "Failed to disable 2FA"));
        return;
      }
      await refreshUser();
      resetTfState();
      setTfMessage("Two-factor authentication disabled");
    } catch {
      setTfError("2FA disable endpoint unavailable");
    } finally {
      setTfLoading(false);
    }
  };

  const handleRegenCodes = async () => {
    resetTfStatus();
    const code = regenCode.trim();
    const isValidTOTP = /^\d{6}$/.test(code);
    const isValidRecovery = /^[0-9a-f]{8}-[0-9a-f]{8}$/i.test(code);
    if (!isValidTOTP && !isValidRecovery) {
      setTfError("Enter a 6-digit code or recovery code");
      return;
    }

    setTfLoading(true);
    try {
      const response = await fetch("/api/auth/2fa/recovery-codes", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ code }),
      });
      const payload = await safeJSON(response);
      if (!response.ok) {
        setTfError(extractError(payload, "Failed to regenerate recovery codes"));
        return;
      }
      const data = payload as { recovery_codes?: string[] } | null;
      setRecoveryCodes(data?.recovery_codes ?? []);
      setShowRegen(false);
      setRegenCode("");
      setTfMessage("Recovery codes regenerated");
    } catch {
      setTfError("Recovery codes endpoint unavailable");
    } finally {
      setTfLoading(false);
    }
  };

  const copySecret = () => {
    void navigator.clipboard.writeText(setupSecret);
    setSecretCopied(true);
    setTimeout(() => setSecretCopied(false), 2000);
  };

  const copyRecoveryCodes = () => {
    void navigator.clipboard.writeText(recoveryCodes.join("\n"));
    setCodesCopied(true);
    setTimeout(() => setCodesCopied(false), 2000);
  };

  return (
    <div className="mt-6 pt-6 border-t border-[var(--line)]">
      <h3 className="text-sm font-medium text-[var(--text)]">Two-Factor Authentication</h3>

      {tfError ? <p className="mt-2 text-sm text-[var(--bad)]">{tfError}</p> : null}
      {tfMessage ? <p className="mt-2 text-sm text-[var(--muted)]">{tfMessage}</p> : null}

      {/* State: 2FA disabled, idle */}
      {!totpEnabled && tfState === "idle" ? (
        <div className="mt-2">
          <p className="text-sm text-[var(--muted)]">
            Two-factor authentication is not enabled. Add an extra layer of security to your account.
          </p>
          <Button variant="primary" className="mt-3" loading={tfLoading} onClick={() => void handleStartSetup()}>
            Enable 2FA
          </Button>
        </div>
      ) : null}

      {/* State: Setting up — show secret + QR + verify input */}
      {tfState === "setting-up" ? (
        <div className="mt-3 space-y-4">
          <p className="text-sm text-[var(--muted)]">
            Scan the QR code below with your authenticator app, or enter the secret key manually.
          </p>

          {setupURI ? (
            <div className="flex justify-center p-4 bg-white rounded-lg w-fit">
              <QRCodeCanvas data={setupURI} size={200} />
            </div>
          ) : null}

          <div>
            <p className="text-xs text-[var(--muted)] mb-1">Secret key (manual entry)</p>
            <div className="flex items-center gap-2">
              <code className="px-3 py-2 bg-[var(--surface)] border border-[var(--line)] rounded-lg text-sm font-mono text-[var(--text)] select-all break-all">
                {setupSecret}
              </code>
              <Button variant="ghost" size="sm" onClick={copySecret}>
                {secretCopied ? "Copied" : "Copy"}
              </Button>
            </div>
          </div>

          <div>
            <p className="text-xs text-[var(--muted)] mb-1">Enter the 6-digit code from your authenticator app</p>
            <div className="flex items-center gap-2">
              <Input
                value={verifyCode}
                onChange={(event) => setVerifyCode(event.target.value.replace(/\D/g, "").slice(0, 6))}
                placeholder="000000"
                maxLength={6}
                className="max-w-[160px] font-mono text-center tracking-widest"
                autoComplete="one-time-code"
              />
              <Button variant="primary" loading={tfLoading} onClick={() => void handleVerify()}>
                Verify &amp; Enable
              </Button>
              <Button variant="ghost" onClick={resetTfState}>
                Cancel
              </Button>
            </div>
          </div>
        </div>
      ) : null}

      {/* State: Just verified — show recovery codes */}
      {tfState === "verifying" && recoveryCodes.length > 0 ? (
        <div className="mt-3 space-y-3">
          <div className="p-3 border border-[var(--warn)]/40 bg-[var(--warn-glow)] rounded-lg">
            <p className="text-sm font-medium text-[var(--text)]">Save your recovery codes</p>
            <p className="text-xs text-[var(--muted)] mt-1">
              These codes can be used to access your account if you lose your authenticator device.
              Each code can only be used once. Store them in a safe place — they will not be shown again.
            </p>
          </div>

          <div className="grid grid-cols-2 gap-2 max-w-sm">
            {recoveryCodes.map((code) => (
              <code key={code} className="px-2 py-1 bg-[var(--surface)] border border-[var(--line)] rounded text-sm font-mono text-[var(--text)] text-center">
                {code}
              </code>
            ))}
          </div>

          <div className="flex items-center gap-2">
            <Button variant="secondary" size="sm" onClick={copyRecoveryCodes}>
              {codesCopied ? "Copied" : "Copy All"}
            </Button>
            <Button variant="ghost" size="sm" onClick={() => { setRecoveryCodes([]); resetTfState(); }}>
              Done
            </Button>
          </div>
        </div>
      ) : null}

      {/* State: 2FA enabled, idle */}
      {totpEnabled && tfState === "idle" ? (
        <div className="mt-2 space-y-3">
          <p className="text-sm text-[var(--ok)]">Two-factor authentication is enabled.</p>

          <div className="flex flex-wrap gap-2">
            {!showDisable ? (
              <Button variant="danger" size="sm" onClick={() => { setShowDisable(true); setShowRegen(false); resetTfStatus(); }}>
                Disable 2FA
              </Button>
            ) : null}
            {!showRegen ? (
              <Button variant="secondary" size="sm" onClick={() => { setShowRegen(true); setShowDisable(false); resetTfStatus(); }}>
                Regenerate Recovery Codes
              </Button>
            ) : null}
          </div>

          {showDisable ? (
            <div>
              <p className="text-xs text-[var(--muted)] mb-1">Enter your authenticator code or a recovery code to disable 2FA</p>
              <div className="flex items-center gap-2">
                <Input
                  value={disableCode}
                  onChange={(event) => setDisableCode(event.target.value.slice(0, 17))}
                  placeholder="Code or recovery code"
                  maxLength={17}
                  className="max-w-[240px] font-mono tracking-wide"
                  autoComplete="one-time-code"
                />
                <Button variant="danger" size="sm" loading={tfLoading} onClick={() => void handleDisable()}>
                  Confirm Disable
                </Button>
                <Button variant="ghost" size="sm" onClick={() => { setShowDisable(false); setDisableCode(""); resetTfStatus(); }}>
                  Cancel
                </Button>
              </div>
            </div>
          ) : null}

          {showRegen ? (
            <div>
              <p className="text-xs text-[var(--muted)] mb-1">Enter your authenticator code to regenerate recovery codes</p>
              <div className="flex items-center gap-2">
                <Input
                  value={regenCode}
                  onChange={(event) => setRegenCode(event.target.value.slice(0, 17))}
                  placeholder="Code or recovery code"
                  maxLength={17}
                  className="max-w-[240px] font-mono tracking-wide"
                  autoComplete="one-time-code"
                />
                <Button variant="primary" size="sm" loading={tfLoading} onClick={() => void handleRegenCodes()}>
                  Regenerate
                </Button>
                <Button variant="ghost" size="sm" onClick={() => { setShowRegen(false); setRegenCode(""); resetTfStatus(); }}>
                  Cancel
                </Button>
              </div>
            </div>
          ) : null}

          {/* Show regenerated recovery codes inline */}
          {recoveryCodes.length > 0 ? (
            <div className="space-y-3">
              <div className="p-3 border border-[var(--warn)]/40 bg-[var(--warn-glow)] rounded-lg">
                <p className="text-sm font-medium text-[var(--text)]">New recovery codes</p>
                <p className="text-xs text-[var(--muted)] mt-1">
                  Your previous recovery codes have been invalidated. Store these new codes in a safe place.
                </p>
              </div>

              <div className="grid grid-cols-2 gap-2 max-w-sm">
                {recoveryCodes.map((code) => (
                  <code key={code} className="px-2 py-1 bg-[var(--surface)] border border-[var(--line)] rounded text-sm font-mono text-[var(--text)] text-center">
                    {code}
                  </code>
                ))}
              </div>

              <Button variant="secondary" size="sm" onClick={copyRecoveryCodes}>
                {codesCopied ? "Copied" : "Copy All"}
              </Button>
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

// ---------------------------------------------------------------------------
// QR Code (client-side, no external service)
// ---------------------------------------------------------------------------

function QRCodeCanvas({ data, size }: { data: string; size: number }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    if (!canvasRef.current || !data) return;
    let cancelled = false;
    import("qrcode")
      .then((QRCode) => {
        if (!cancelled && canvasRef.current) {
          return QRCode.toCanvas(canvasRef.current, data, {
            width: size,
            margin: 2,
            color: { dark: "#000000", light: "#ffffff" },
          });
        }
      })
      .catch(() => {
        // QR rendering failed — canvas remains blank.
      });
    return () => { cancelled = true; };
  }, [data, size]);

  return <canvas ref={canvasRef} />;
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


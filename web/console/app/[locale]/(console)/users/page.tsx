"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { Button } from "../../../components/ui/Button";
import { useAuth } from "../../../contexts/AuthContext";
import { useHubUsers } from "../../../hooks/useHubUsers";
import type { HubUser } from "../../../hooks/useHubUsers";
import { UsersTable } from "./UsersTable";
import { CreateUserDialog } from "./CreateUserDialog";
import { EditRoleDialog } from "./EditRoleDialog";
import { ResetPasswordDialog } from "./ResetPasswordDialog";
import { DeleteUserDialog } from "./DeleteUserDialog";
import { useToast } from "../../../contexts/ToastContext";
import { AccountSecurityCard } from "../settings/components/AccountSecurityCard";

type Dialog =
  | { type: "create" }
  | { type: "editRole"; user: HubUser }
  | { type: "resetPassword"; user: HubUser }
  | { type: "delete"; user: HubUser }
  | null;

export default function UsersPage() {
  const t = useTranslations("users");
  const { user: currentUser } = useAuth();
  const { users, loading, error, createUser, updateRole, resetPassword, deleteUser, revokeSessions } = useHubUsers();
  const { addToast } = useToast();
  const [dialog, setDialog] = useState<Dialog>(null);

  const canManage = currentUser?.role === "owner" || currentUser?.role === "admin";

  const handleCreate = async (payload: { username: string; password: string; role: string }) => {
    await createUser(payload);
    addToast("success", `User ${payload.username} created.`);
    setDialog(null);
  };

  const handleEditRole = async (user: HubUser, role: string) => {
    await updateRole(user.id, role);
    addToast("success", `Role updated for ${user.username}.`);
    setDialog(null);
  };

  const handleResetPassword = async (user: HubUser, password: string) => {
    await resetPassword(user.id, password);
    addToast("success", `Password reset for ${user.username}.`);
    setDialog(null);
  };

  const handleDelete = async (user: HubUser) => {
    await deleteUser(user.id);
    addToast("success", `User ${user.username} deleted.`);
    setDialog(null);
  };

  const handleRevokeSessions = async (user: HubUser) => {
    await revokeSessions(user.id);
    addToast("success", `Sessions revoked for ${user.username}.`);
  };

  const sortedUsers = [...users].sort((a, b) => a.username.localeCompare(b.username));

  return (
    <>
      <PageHeader title={t("title")} subtitle={t("subtitle")} />

      <AccountSecurityCard />

      {canManage && (
        <>
          <div className="flex items-center justify-between mb-6 mt-2">
            <p className="text-xs font-semibold uppercase tracking-wider text-[var(--muted)]">{t("userManagement")}</p>
            <Button variant="primary" size="sm" onClick={() => setDialog({ type: "create" })}>
              <Plus size={14} />
              {t("addUser")}
            </Button>
          </div>

          {error ? (
            <p className="mb-4 text-sm text-[var(--bad)]">{error}</p>
          ) : null}

          <UsersTable
            users={sortedUsers}
            loading={loading}
            currentUserId={currentUser?.id ?? ""}
            onEditRole={(user) => setDialog({ type: "editRole", user })}
            onResetPassword={(user) => setDialog({ type: "resetPassword", user })}
            onRevokeSessions={(user) => { void handleRevokeSessions(user); }}
            onDelete={(user) => setDialog({ type: "delete", user })}
          />

          <CreateUserDialog
            open={dialog?.type === "create"}
            onClose={() => setDialog(null)}
            onConfirm={handleCreate}
          />

          <EditRoleDialog
            user={dialog?.type === "editRole" ? dialog.user : null}
            onClose={() => setDialog(null)}
            onConfirm={handleEditRole}
          />

          <ResetPasswordDialog
            user={dialog?.type === "resetPassword" ? dialog.user : null}
            onClose={() => setDialog(null)}
            onConfirm={handleResetPassword}
          />

          <DeleteUserDialog
            user={dialog?.type === "delete" ? dialog.user : null}
            onClose={() => setDialog(null)}
            onConfirm={handleDelete}
          />
        </>
      )}
    </>
  );
}

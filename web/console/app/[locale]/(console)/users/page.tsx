"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { Button } from "../../../components/ui/Button";
import { useAuth } from "../../../contexts/AuthContext";
import { useToast } from "../../../contexts/ToastContext";
import { useHubUsers } from "../../../hooks/useHubUsers";
import type { HubUser } from "../../../hooks/useHubUsers";
import { canManageUserAccount, hasAdminRole } from "../../../lib/roles";
import { AccountSecurityCard } from "../settings/components/AccountSecurityCard";
import { CreateUserDialog } from "./CreateUserDialog";
import { DeleteUserDialog, SignOutUserDialog } from "./DeleteUserDialog";
import { EditRoleDialog } from "./EditRoleDialog";
import { ResetPasswordDialog } from "./ResetPasswordDialog";
import { UserList, type UserAction } from "./UserList";

type Dialog = { type: "create" } | { type: UserAction; user: HubUser } | null;

/**
 * Settings › Users. Admins see the user list (Add user; each row's ⋯ changes the role, resets a local password, signs
 * the user out everywhere or removes them). Everyone keeps their own password and two-factor sign-in below it.
 */
export default function UsersPage() {
  const t = useTranslations("users");
  const tc = useTranslations("common");
  const { user: currentUser } = useAuth();
  const canManage = hasAdminRole(currentUser?.role);
  const { users, loading, error, refresh, createUser, updateRole, resetPassword, deleteUser, revokeSessions } = useHubUsers(canManage);
  const { addToast } = useToast();
  const [dialog, setDialog] = useState<Dialog>(null);
  const close = () => setDialog(null);
  const target = (type: UserAction) => (dialog && dialog.type === type ? dialog.user : null);
  const you = (user: HubUser | null) => Boolean(user && user.id === currentUser?.id);

  const done = (message: string) => {
    addToast("success", message);
    setDialog(null);
  };

  return (
    <>
      <PageHeader
        title={t("title")}
        action={canManage ? (
          <Button variant="primary" onClick={() => setDialog({ type: "create" })}>
            <Plus size={14} aria-hidden="true" />
            {t("addUser")}
          </Button>
        ) : undefined}
      />

      {canManage ? (
        <>
          {error ? (
            <p className="pp-err pp-load" role="alert">
              {t("list.loadFailed")}
              <button type="button" className="pp-link" onClick={() => { void refresh(); }}>{tc("retry")}</button>
            </p>
          ) : null}
          <UserList
            users={users}
            loading={loading}
            currentUserId={currentUser?.id ?? ""}
            canManage={(user) => canManageUserAccount(currentUser, user)}
            onAction={(type, user) => setDialog({ type, user })}
          />
        </>
      ) : null}

      <div className={canManage ? "pp-own" : undefined}>
        <AccountSecurityCard />
      </div>

      <CreateUserDialog
        open={dialog?.type === "create"}
        callerRole={currentUser?.role}
        onClose={close}
        onConfirm={async (payload) => {
          await createUser(payload);
          addToast("success", t("toast.added", { name: payload.username }));
        }}
      />
      <EditRoleDialog
        user={target("role")}
        callerRole={currentUser?.role}
        onClose={close}
        onConfirm={async (user, role) => {
          await updateRole(user.id, role);
          done(t("toast.role", { name: user.username, role: t(`roles.${role}`) }));
        }}
      />
      <ResetPasswordDialog
        user={target("password")}
        onClose={close}
        onConfirm={async (user, password) => {
          await resetPassword(user.id, password);
          done(t("toast.password", { name: user.username }));
        }}
      />
      <SignOutUserDialog
        user={target("signOut")}
        you={you(target("signOut"))}
        onClose={close}
        onConfirm={async (user) => {
          await revokeSessions(user.id);
          done(t("toast.signedOut", { name: user.username }));
        }}
      />
      <DeleteUserDialog
        user={target("remove")}
        onClose={close}
        onConfirm={async (user) => {
          await deleteUser(user.id);
          done(t("toast.removed", { name: user.username }));
        }}
      />
    </>
  );
}

"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import type { HubUser } from "../../../hooks/useHubUsers";
import { PeopleDialog, errorText } from "./PeopleDialog";

type UserConfirmProps = {
  user: HubUser | null;
  you?: boolean;
  onClose: () => void;
  onConfirm: (user: HubUser) => Promise<void>;
};

/** Asks once, shows the hub's refusal in place, and stays open until the action is done. */
function useConfirm(user: HubUser | null, onConfirm: (user: HubUser) => Promise<void>, fallback: string) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    setError("");
    setBusy(false);
  }, [user]);
  const run = async () => {
    if (!user) return;
    setBusy(true);
    setError("");
    try {
      await onConfirm(user);
    } catch (err) {
      setError(errorText(err, fallback));
      setBusy(false);
    }
  };
  return { error, busy, run };
}

/** Remove a user: the account and its sessions end. */
export function DeleteUserDialog({ user, onClose, onConfirm }: UserConfirmProps) {
  const t = useTranslations("users");
  const { error, busy, run } = useConfirm(user, onConfirm, t("errors.removeFailed"));
  const name = user?.username ?? "";
  return (
    <PeopleDialog
      open={Boolean(user)}
      title={t("remove.title", { name })}
      onClose={onClose}
      onSubmit={() => { void run(); }}
      submitLabel={t("remove.submit")}
      tone="danger"
      busy={busy}
      error={error}
    >
      <p className="pp-say">{t("remove.body", { name })}</p>
    </PeopleDialog>
  );
}

/** Sign out everywhere: every session of the user ends, this one too when it is your own account. */
export function SignOutUserDialog({ user, you = false, onClose, onConfirm }: UserConfirmProps) {
  const t = useTranslations("users");
  const { error, busy, run } = useConfirm(user, onConfirm, t("errors.signOutFailed"));
  const name = user?.username ?? "";
  return (
    <PeopleDialog
      open={Boolean(user)}
      title={you ? t("signOut.titleYou") : t("signOut.title", { name })}
      onClose={onClose}
      onSubmit={() => { void run(); }}
      submitLabel={t("signOut.submit")}
      busy={busy}
      error={error}
    >
      <p className="pp-say">{you ? t("signOut.bodyYou") : t("signOut.body", { name })}</p>
    </PeopleDialog>
  );
}

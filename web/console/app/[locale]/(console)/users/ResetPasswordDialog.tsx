"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Input } from "../../../components/ui/Input";
import type { HubUser } from "../../../hooks/useHubUsers";
import { Field, PeopleDialog, errorText } from "./PeopleDialog";

type ResetPasswordDialogProps = {
  user: HubUser | null;
  onClose: () => void;
  onConfirm: (user: HubUser, password: string) => Promise<void>;
};

type Errors = { pass?: string; pass2?: string; hub?: string };

/** Reset password for a local account: the new password typed twice. Nothing is kept after the dialog closes. */
export function ResetPasswordDialog({ user, onClose, onConfirm }: ResetPasswordDialogProps) {
  const t = useTranslations("users");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [errors, setErrors] = useState<Errors>({});
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setPassword("");
    setConfirmPassword("");
    setErrors({});
    setSaving(false);
  }, [user]);

  const submit = async () => {
    if (!user) return;
    const found: Errors = password.length < 8
      ? { pass: t("errors.passShort") }
      : password !== confirmPassword ? { pass2: t("errors.passMismatch") } : {};
    setErrors(found);
    if (found.pass || found.pass2) return;
    setSaving(true);
    setErrors({});
    try {
      await onConfirm(user, password);
    } catch (err) {
      setErrors({ hub: errorText(err, t("errors.passwordFailed")) });
      setSaving(false);
    }
  };

  return (
    <PeopleDialog
      open={Boolean(user)}
      title={t("password.title", { name: user?.username ?? "" })}
      onClose={onClose}
      onSubmit={() => { void submit(); }}
      submitLabel={t("password.submit")}
      busy={saving}
      error={errors.hub}
    >
      <div className="pp-grid2">
        <Field id="pp-rp-pass" label={t("fields.newPassword")} error={errors.pass}>
          <Input
            id="pp-rp-pass"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            maxLength={256}
            autoComplete="new-password"
            autoFocus
            disabled={saving}
            error={Boolean(errors.pass)}
            aria-invalid={errors.pass ? true : undefined}
            aria-describedby={errors.pass ? "pp-rp-pass-err" : undefined}
          />
        </Field>
        <Field id="pp-rp-pass2" label={t("fields.repeatPassword")} error={errors.pass2}>
          <Input
            id="pp-rp-pass2"
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            maxLength={256}
            autoComplete="new-password"
            disabled={saving}
            error={Boolean(errors.pass2)}
            aria-invalid={errors.pass2 ? true : undefined}
            aria-describedby={errors.pass2 ? "pp-rp-pass2-err" : undefined}
          />
        </Field>
      </div>
    </PeopleDialog>
  );
}

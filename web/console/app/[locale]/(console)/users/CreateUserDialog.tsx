"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Input, Select } from "../../../components/ui/Input";
import { assignableUserRoles } from "../../../lib/roles";
import { Field, PeopleDialog, errorText } from "./PeopleDialog";

type CreateUserDialogProps = {
  open: boolean;
  callerRole?: string;
  onClose: () => void;
  onConfirm: (payload: { username: string; password: string; role: string }) => Promise<void>;
};

const USERNAME_RE = /^[a-zA-Z0-9\-._]+$/;

type Errors = { name?: string; pass?: string; pass2?: string; hub?: string };

/** Add user: a username, a role the caller may give, and a password typed twice. Passwords are cleared on close. */
export function CreateUserDialog({ open, callerRole, onClose, onConfirm }: CreateUserDialogProps) {
  const t = useTranslations("users");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [role, setRole] = useState("viewer");
  const [errors, setErrors] = useState<Errors>({});
  const [creating, setCreating] = useState(false);

  const reset = () => {
    setUsername("");
    setPassword("");
    setConfirmPassword("");
    setRole("viewer");
    setErrors({});
    setCreating(false);
  };

  const check = (name: string): Errors => {
    const next: Errors = {};
    if (!name) next.name = t("errors.nameEmpty");
    else if (!USERNAME_RE.test(name)) next.name = t("errors.nameChars");
    else if (name.length > 32) next.name = t("errors.nameLong");
    if (password.length < 8) next.pass = t("errors.passShort");
    else if (password !== confirmPassword) next.pass2 = t("errors.passMismatch");
    return next;
  };

  const submit = async () => {
    const name = username.trim().toLowerCase();
    const found = check(name);
    setErrors(found);
    if (Object.keys(found).length) return;
    setCreating(true);
    try {
      await onConfirm({ username: name, password, role });
      reset();
      onClose();
    } catch (err) {
      setErrors({ hub: errorText(err, t("errors.addFailed")) });
      setCreating(false);
    }
  };

  const close = () => {
    reset();
    onClose();
  };

  return (
    <PeopleDialog
      open={open}
      title={t("add.title")}
      onClose={close}
      onSubmit={() => { void submit(); }}
      submitLabel={t("add.submit")}
      tone="main"
      busy={creating}
      error={errors.hub}
    >
      <div className="pp-grid2">
        <Field id="pp-u-name" label={t("fields.username")} error={errors.name}>
          <Input
            id="pp-u-name"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="ops-viewer"
            maxLength={32}
            autoComplete="off"
            autoFocus
            disabled={creating}
            error={Boolean(errors.name)}
            aria-invalid={errors.name ? true : undefined}
            aria-describedby={errors.name ? "pp-u-name-err" : undefined}
          />
        </Field>
        <Field id="pp-u-role" label={t("fields.role")}>
          <Select id="pp-u-role" name="role" value={role} onChange={(e) => setRole(e.target.value)} disabled={creating} className="w-full">
            {assignableUserRoles(callerRole).map((value) => (
              <option key={value} value={value}>{t(`roles.${value}`)}</option>
            ))}
          </Select>
        </Field>
        <Field id="pp-u-pass" label={t("fields.password")} error={errors.pass}>
          <Input
            id="pp-u-pass"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            maxLength={256}
            autoComplete="new-password"
            disabled={creating}
            error={Boolean(errors.pass)}
            aria-invalid={errors.pass ? true : undefined}
            aria-describedby={errors.pass ? "pp-u-pass-err" : undefined}
          />
        </Field>
        <Field id="pp-u-pass2" label={t("fields.repeatPassword")} error={errors.pass2}>
          <Input
            id="pp-u-pass2"
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            maxLength={256}
            autoComplete="new-password"
            disabled={creating}
            error={Boolean(errors.pass2)}
            aria-invalid={errors.pass2 ? true : undefined}
            aria-describedby={errors.pass2 ? "pp-u-pass2-err" : undefined}
          />
        </Field>
      </div>
    </PeopleDialog>
  );
}

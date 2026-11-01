"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Select } from "../../../components/ui/Input";
import type { HubUser } from "../../../hooks/useHubUsers";
import { assignableUserRoles } from "../../../lib/roles";
import { Field, PeopleDialog, errorText } from "./PeopleDialog";

type EditRoleDialogProps = {
  user: HubUser | null;
  callerRole?: string;
  onClose: () => void;
  onConfirm: (user: HubUser, role: string) => Promise<void>;
};

/** Change role: the roles the caller may give this user, the current one picked. */
export function EditRoleDialog({ user, callerRole, onClose, onConfirm }: EditRoleDialogProps) {
  const t = useTranslations("users");
  const [role, setRole] = useState("viewer");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!user) return;
    setRole(user.role === "owner" ? "admin" : user.role || "viewer");
    setError("");
    setSaving(false);
  }, [user]);

  const submit = async () => {
    if (!user) return;
    setSaving(true);
    setError("");
    try {
      await onConfirm(user, role);
    } catch (err) {
      setError(errorText(err, t("errors.roleFailed")));
      setSaving(false);
    }
  };

  return (
    <PeopleDialog
      open={Boolean(user)}
      title={t("role.title", { name: user?.username ?? "" })}
      onClose={onClose}
      onSubmit={() => { void submit(); }}
      submitLabel={t("role.submit")}
      busy={saving}
      error={error}
      disabled={role === user?.role}
    >
      <Field id="pp-role" label={t("fields.role")}>
        <Select id="pp-role" name="role" value={role} onChange={(e) => setRole(e.target.value)} disabled={saving} className="w-full">
          {user
            ? assignableUserRoles(callerRole, user.role).map((value) => (
                <option key={value} value={value}>{t(`roles.${value}`)}</option>
              ))
            : null}
        </Select>
      </Field>
    </PeopleDialog>
  );
}

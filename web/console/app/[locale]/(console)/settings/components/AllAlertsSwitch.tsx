"use client";

import { useTranslations } from "next-intl";
import { Field, Switch } from "./SettingsKit";

/**
 * "Send every alert here": on, the channel gets every alert; off, only the alerts an alert route sends it. Push
 * channels keep their own route, so the dialogs leave the switch out for them.
 */
export function AllAlertsSwitch({ checked, onChange, disabled = false }: {
  checked: boolean;
  onChange: (next: boolean) => void;
  disabled?: boolean;
}) {
  const t = useTranslations("notifications");
  return (
    <Field label={t("allAlerts.label")} hint={t("allAlerts.hint")} wide>
      <Switch checked={checked} onChange={onChange} label={t("allAlerts.label")} disabled={disabled} />
    </Field>
  );
}

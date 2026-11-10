"use client";

import { useTranslations } from "next-intl";
import { useCallback } from "react";

import { Select } from "../ui/Input";
import type { CredentialProfile } from "../../hooks/useCredentialProfiles";

/** Kinds with a name under security.logins.kinds; a kind the console does not know shows as the hub sent it. */
const namedKinds = new Set([
  "ssh_password",
  "ssh_private_key",
  "hub_ssh_identity",
  "vnc_password",
  "proxmox_api_token",
  "proxmox_password",
  "pbs_api_token",
  "portainer_api_key",
  "truenas_api_key",
  "homeassistant_token",
  "telnet_password",
  "rdp_password",
  "ftp_password",
  "smb_credentials",
  "webdav_credentials",
]);

export const sshCredentialKinds = ["ssh_password", "ssh_private_key", "hub_ssh_identity"];

/** A saved login's type ("SSH password", "Proxmox API token") in the person's language. */
export function useCredentialKindLabel(): (kind: string) => string {
  const t = useTranslations("security.logins.kinds");
  return useCallback((kind: string) => (namedKinds.has(kind) ? t(kind) : kind), [t]);
}

type CredentialProfileSelectProps = {
  id: string;
  label?: string;
  ariaLabel?: string;
  value: string;
  onChange: (value: string) => void;
  profiles: CredentialProfile[];
  loading: boolean;
  error: string | null;
  allowedKinds?: readonly string[];
  disabled?: boolean;
  className?: string;
  emptyLabel?: string;
};

export function CredentialProfileSelect({
  id,
  label,
  ariaLabel,
  value,
  onChange,
  profiles,
  loading,
  error,
  allowedKinds,
  disabled = false,
  className = "",
  emptyLabel,
}: CredentialProfileSelectProps) {
  const t = useTranslations("security.logins");
  const kindLabel = useCredentialKindLabel();
  const allowed = allowedKinds ? new Set(allowedKinds) : null;
  const options = profiles.filter((profile) => !allowed || allowed.has(profile.kind));
  const selectedAvailable = value === "" || options.some((profile) => profile.id === value);
  const statusID = `${id}-status`;

  return (
    <div className={className}>
      {label ? <label htmlFor={id} className="mb-1 block text-xs font-medium text-[var(--muted)]">{label}</label> : null}
      <Select
        id={id}
        aria-label={ariaLabel}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled || loading}
        aria-describedby={error ? statusID : undefined}
        error={Boolean(error)}
        className="w-full"
      >
        <option value="">{loading ? t("loading") : emptyLabel ?? t("none")}</option>
        {!selectedAvailable ? <option value={value}>{t("unavailable")}</option> : null}
        {options.map((profile) => {
          const kind = kindLabel(profile.kind);
          return (
            <option key={profile.id} value={profile.id}>
              {profile.username ? t("optionUser", { name: profile.name, kind, username: profile.username }) : t("option", { name: profile.name, kind })}
            </option>
          );
        })}
      </Select>
      {error ? <p id={statusID} role="alert" className="mt-1 text-[11px] text-[var(--bad)]">{t("loadFailed")}</p> : null}
    </div>
  );
}

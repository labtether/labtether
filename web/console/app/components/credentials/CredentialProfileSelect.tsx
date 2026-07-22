"use client";

import { Select } from "../ui/Input";
import type { CredentialProfile } from "../../hooks/useCredentialProfiles";

export const credentialKindLabels: Record<string, string> = {
  ssh_password: "SSH password",
  ssh_private_key: "SSH private key",
  hub_ssh_identity: "Hub SSH identity",
  vnc_password: "VNC / ARD password",
  proxmox_api_token: "Proxmox API token",
  proxmox_password: "Proxmox password",
  pbs_api_token: "PBS API token",
  portainer_api_key: "Portainer API key",
  truenas_api_key: "TrueNAS API key",
  homeassistant_token: "Home Assistant token",
  telnet_password: "Telnet password",
  rdp_password: "RDP password",
  ftp_password: "FTP password",
  smb_credentials: "SMB credentials",
  webdav_credentials: "WebDAV credentials",
};

export const sshCredentialKinds = ["ssh_password", "ssh_private_key", "hub_ssh_identity"];

export function credentialProfileLabel(profile: CredentialProfile): string {
  const kind = credentialKindLabels[profile.kind] ?? profile.kind;
  return profile.username
    ? `${profile.name} — ${kind} (${profile.username})`
    : `${profile.name} — ${kind}`;
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
  emptyLabel = "No credential profile",
}: CredentialProfileSelectProps) {
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
        <option value="">{loading ? "Loading credential profiles…" : emptyLabel}</option>
        {!selectedAvailable ? <option value={value}>Previously selected profile (unavailable)</option> : null}
        {options.map((profile) => (
          <option key={profile.id} value={profile.id}>{credentialProfileLabel(profile)}</option>
        ))}
      </Select>
      {error ? <p id={statusID} role="alert" className="mt-1 text-[11px] text-[var(--bad)]">{error}</p> : null}
    </div>
  );
}

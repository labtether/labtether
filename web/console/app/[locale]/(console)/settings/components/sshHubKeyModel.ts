export const SSH_HUB_KEY_ROTATION_CONFIRMATION = "ROTATE";
export const SSH_HUB_KEY_ROTATION_REASON_MAX_LENGTH = 256;

export type SSHHubKeyType = "ed25519" | "rsa";

export type SSHHubKeyInfo = {
  public_key: string;
  key_type: SSHHubKeyType;
  fingerprint_sha256: string;
  agents_updated?: number;
  agents_total?: number;
  old_key_removal_failures?: number;
  warning?: string;
};

export type SSHHubKeyRotationPayload = {
  key_type: SSHHubKeyType;
  reason: string;
  confirm: typeof SSH_HUB_KEY_ROTATION_CONFIRMATION;
};

export function parseSSHHubKeyInfo(value: unknown): SSHHubKeyInfo | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  const record = value as Record<string, unknown>;
  if (
    typeof record.public_key !== "string"
    || (record.key_type !== "ed25519" && record.key_type !== "rsa")
    || typeof record.fingerprint_sha256 !== "string"
  ) {
    return null;
  }
  return {
    public_key: record.public_key,
    key_type: record.key_type,
    fingerprint_sha256: record.fingerprint_sha256,
    agents_updated: typeof record.agents_updated === "number" ? record.agents_updated : undefined,
    agents_total: typeof record.agents_total === "number" ? record.agents_total : undefined,
    old_key_removal_failures: typeof record.old_key_removal_failures === "number"
      ? record.old_key_removal_failures
      : undefined,
    warning: typeof record.warning === "string" ? record.warning : undefined,
  };
}

export function sshHubKeyErrorMessage(value: unknown, fallback: string): string {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    const error = (value as Record<string, unknown>).error;
    if (typeof error === "string" && error.trim() !== "") return error.trim();
  }
  return fallback;
}

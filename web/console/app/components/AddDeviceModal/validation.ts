const PROXMOX_TOKEN_ID_RE = /^[^@\s]+@[^!\s]+![^\s!]+$/;

/**
 * The validators return a message key under `auth.addDevice.validation`, or "" when the value is fine.
 * The setup steps turn the key into text with `validationText`.
 */
export type ValidationKey =
  | ""
  | "urlRequired"
  | "urlInvalid"
  | "urlScheme"
  | "urlHost"
  | "interval"
  | "tokenIdRequired"
  | "tokenIdFormat"
  | "tokenIdLength";

export function validateHTTPSOrHTTPURL(raw: string): ValidationKey {
  const value = raw.trim();
  if (!value) {
    return "urlRequired";
  }
  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    return "urlInvalid";
  }
  const protocol = parsed.protocol.toLowerCase();
  if (protocol !== "https:" && protocol !== "http:") {
    return "urlScheme";
  }
  if (!parsed.hostname.trim()) {
    return "urlHost";
  }
  return "";
}

export function validatePollIntervalSeconds(value: number): ValidationKey {
  if (!Number.isFinite(value) || value < 15 || value > 3600) {
    return "interval";
  }
  return "";
}

export function validateProxmoxTokenID(raw: string): ValidationKey {
  const value = raw.trim();
  if (!value) {
    return "tokenIdRequired";
  }
  if (!PROXMOX_TOKEN_ID_RE.test(value)) {
    return "tokenIdFormat";
  }
  return "";
}

export function validatePortainerTokenID(raw: string): ValidationKey {
  const value = raw.trim();
  if (value.length > 255) {
    return "tokenIdLength";
  }
  return "";
}

export function baseURLHostLabel(raw: string): string {
  try {
    const parsed = new URL(raw.trim());
    return parsed.hostname || "";
  } catch {
    return "";
  }
}

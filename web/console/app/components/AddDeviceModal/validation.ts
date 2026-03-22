const PROXMOX_TOKEN_ID_RE = /^[^@\s]+@[^!\s]+![^\s!]+$/;

export function validateHTTPSOrHTTPURL(raw: string, label = "Base URL"): string {
  const value = raw.trim();
  if (!value) {
    return `${label} is required.`;
  }
  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    return `${label} must be a valid URL.`;
  }
  const protocol = parsed.protocol.toLowerCase();
  if (protocol !== "https:" && protocol !== "http:") {
    return `${label} must start with http:// or https://.`;
  }
  if (!parsed.hostname.trim()) {
    return `${label} must include a hostname.`;
  }
  return "";
}

export function validatePollIntervalSeconds(value: number): string {
  if (!Number.isFinite(value)) {
    return "Poll interval must be a number.";
  }
  if (value < 15 || value > 3600) {
    return "Poll interval must be between 15 and 3600 seconds.";
  }
  return "";
}

export function validateProxmoxTokenID(raw: string): string {
  const value = raw.trim();
  if (!value) {
    return "Token ID is required.";
  }
  if (!PROXMOX_TOKEN_ID_RE.test(value)) {
    return "Token ID must look like user@realm!tokenname.";
  }
  return "";
}

export function validatePortainerTokenID(raw: string): string {
  const value = raw.trim();
  if (!value) {
    return "";
  }
  if (value.length > 255) {
    return "Token ID must be 255 characters or fewer.";
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

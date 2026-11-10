"use client";

import { type HubConnectionCandidate } from "../../hooks/useEnrollment";
import { trimTrailingSlashes } from "../../../lib/trimChars";
import type { AddDeviceT } from "./addDeviceText";

export type Platform = "linux" | "macos" | "windows";

export type LinuxDockerMode = "auto" | "true" | "false";

export type FilesRootMode = "home" | "full";

export type LinuxInstallOptions = {
  dockerEnabled: LinuxDockerMode;
  dockerEndpoint: string;
  dockerDiscoveryIntervalSec: string;
  filesRootMode: FilesRootMode;
  autoInstallVNC: boolean;
  autoUpdateEnabled: boolean;
  forceUpdate: boolean;
  includeEnrollmentToken: boolean;
};

export const defaultLinuxInstallOptions: LinuxInstallOptions = {
  dockerEnabled: "auto",
  dockerEndpoint: "/var/run/docker.sock",
  dockerDiscoveryIntervalSec: "30",
  filesRootMode: "home",
  autoInstallVNC: true,
  autoUpdateEnabled: true,
  forceUpdate: false,
  includeEnrollmentToken: true,
};

export function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\"'\"'`)}'`;
}

export function powershellQuote(value: string): string {
  return `'${value.replace(/'/g, "''")}'`;
}

export function normalizeHubURL(raw: string): string {
  return trimTrailingSlashes(raw.trim());
}

export function pinnedCAFingerprint(
  hubURL: string,
  candidate: HubConnectionCandidate | null | undefined,
): string {
  if (
    candidate?.bootstrap_strategy !== "pinned_ca_bootstrap" ||
    !candidate.bootstrap_url
  ) {
    return "";
  }

  try {
    const base = new URL(hubURL);
    const bootstrap = new URL(candidate.bootstrap_url);
    const expectedPath = `${trimTrailingSlashes(base.pathname)}/api/v1/agent/bootstrap.sh`;
    const fingerprint =
      bootstrap.searchParams.get("ca_fingerprint_sha256") ?? "";
    if (
      base.protocol !== "https:" ||
      base.username ||
      base.password ||
      bootstrap.origin !== base.origin ||
      bootstrap.pathname !== expectedPath ||
      !/^[a-fA-F0-9]{64}$/.test(fingerprint)
    ) {
      return "";
    }
    return fingerprint.toLowerCase();
  } catch {
    return "";
  }
}

export function parseBoundedDecimalInt(
  raw: string,
  min: number,
  max: number,
  fallback: number,
): number {
  const trimmed = raw.trim();
  if (!/^[0-9]+$/.test(trimmed)) {
    return fallback;
  }
  const parsed = Number(trimmed);
  return Number.isInteger(parsed) && parsed >= min && parsed <= max
    ? parsed
    : fallback;
}

// The bash commands below run their script from a here-document, so bash's own input is the script. `3<&0` keeps
// the terminal on descriptor 3 and the token prompt reads from there; reading stdin would take the script's next line.
export function manualInstallCommand(
  platform: Platform,
  wsURL: string,
): string {
  if (platform === "windows") {
    return `$tokenFile = Join-Path $env:TEMP "labtether-enrollment-token-$PID"\n$secureToken = Read-Host "Paste enrollment token" -AsSecureString\n$pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureToken)\ntry {\n  [IO.File]::WriteAllText($tokenFile, [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer))\n} finally {\n  [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)\n}\n$env:LABTETHER_ENROLLMENT_TOKEN_FILE=$tokenFile\n$env:LABTETHER_WS_URL=${powershellQuote(wsURL)}\ntry {\n  .\\labtether-agent.exe\n} finally {\n  Remove-Item -Force -ErrorAction SilentlyContinue $tokenFile\n  Remove-Item Env:LABTETHER_ENROLLMENT_TOKEN_FILE -ErrorAction SilentlyContinue\n}`;
  }
  return `bash 3<&0 <<'LABTETHER_AGENT'\nset -euo pipefail\numask 077\ntoken_file="$(mktemp)"\ntrap 'rm -f "$token_file"' EXIT\nread -r -s -p 'Paste enrollment token: ' token <&3\nprintf '\\n'\nprintf '%s\\n' "$token" > "$token_file"\nunset token\nLABTETHER_ENROLLMENT_TOKEN_FILE="$token_file" \\\n  LABTETHER_WS_URL=${shellQuote(wsURL)} \\\n  ./labtether-agent\nLABTETHER_AGENT`;
}

const TRUST_MODE_KEYS = new Map([
  ["public_tls", "publicTls"],
  ["custom_tls", "customTls"],
  ["labtether_ca", "labtetherCa"],
  ["plain_http", "plainHttp"],
]);

/**
 * How agents trust the selected address, as a short label and one line of detail. The detail comes from the
 * console's messages so it follows the language; a trust mode it does not know falls back to the hub's text.
 */
export function trustModeText(
  t: AddDeviceT,
  candidate: HubConnectionCandidate | null | undefined,
  platform: Platform = "linux",
): { label: string; reason: string } | null {
  const key = TRUST_MODE_KEYS.get(candidate?.trust_mode ?? "");
  if (key) {
    // Only the Linux install command adds the Hub's CA by itself. Mac and Windows setup show its fingerprint and ask.
    const reasonKey = key === "labtetherCa" && platform !== "linux" ? "labtetherCaApp" : key;
    return { label: t(`agent.trust.${key}`), reason: t(`agent.trustReason.${reasonKey}`) };
  }
  const reason = candidate?.preferred_reason ?? "";
  return reason ? { label: t("agent.trust.other"), reason } : null;
}

export function linuxInstallerCommand(
  hubURL: string,
  options: LinuxInstallOptions,
  candidate?: HubConnectionCandidate | null,
): string {
  const base = normalizeHubURL(hubURL);
  const expectedCAFingerprint = pinnedCAFingerprint(base, candidate);
  const usePinnedBootstrap = expectedCAFingerprint !== "";
  const managedCAPath = "/etc/labtether/ca.crt";
  const defaults = defaultLinuxInstallOptions;
  // The agent treats settings written at install time as fixed and ignores
  // Hub changes to them. Pass a Docker or file-scope flag only when the user
  // changed it from the default, so the Hub can still manage the rest.
  const flags: string[] = [];
  if (options.dockerEnabled !== defaults.dockerEnabled) {
    flags.push(`--docker-enabled ${options.dockerEnabled}`);
  }
  if (options.filesRootMode !== defaults.filesRootMode) {
    flags.push(`--files-root-mode ${options.filesRootMode}`);
  }
  flags.push(`--auto-update ${options.autoUpdateEnabled ? "true" : "false"}`);

  if (options.autoInstallVNC) {
    flags.push("--install-vnc-prereqs");
  }

  if (options.dockerEnabled !== "false") {
    const endpoint = options.dockerEndpoint.trim() || defaults.dockerEndpoint;
    if (endpoint !== defaults.dockerEndpoint) {
      flags.push(`--docker-endpoint ${shellQuote(endpoint)}`);
    }

    const defaultInterval = Number(defaults.dockerDiscoveryIntervalSec);
    const boundedInterval = parseBoundedDecimalInt(
      options.dockerDiscoveryIntervalSec,
      5,
      3600,
      defaultInterval,
    );
    if (boundedInterval !== defaultInterval) {
      flags.push(`--docker-discovery-interval ${boundedInterval}`);
    }
  }

  if (options.forceUpdate) {
    flags.push("--force-update");
  }

  if (usePinnedBootstrap) {
    flags.unshift(`--tls-ca-file ${shellQuote(managedCAPath)}`);
  }

  const installerDownload = usePinnedBootstrap
    ? `ca_file="$(mktemp)"
expected_ca_fingerprint=${shellQuote(expectedCAFingerprint)}
curl -kfsSL ${shellQuote(`${base}/api/v1/ca.crt`)} -o "$ca_file"
if ! command -v openssl >/dev/null 2>&1; then
  echo 'Error: openssl is required to verify the LabTether CA.' >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual_ca_fingerprint="$(openssl x509 -in "$ca_file" -outform DER | sha256sum | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual_ca_fingerprint="$(openssl x509 -in "$ca_file" -outform DER | shasum -a 256 | awk '{print $1}')"
else
  echo 'Error: sha256sum or shasum is required to verify the LabTether CA.' >&2
  exit 1
fi
actual_ca_fingerprint="$(printf '%s' "$actual_ca_fingerprint" | tr '[:upper:]' '[:lower:]')"
if [[ "$actual_ca_fingerprint" != "$expected_ca_fingerprint" ]]; then
  echo 'Error: LabTether CA fingerprint mismatch; refusing to run downloaded content.' >&2
  exit 1
fi
sudo install -d -m 0755 /etc/labtether
sudo install -m 0644 "$ca_file" ${shellQuote(managedCAPath)}
curl --cacert "$ca_file" -fsSL ${shellQuote(`${base}/install.sh`)} -o "$installer"`
    : `curl -fsSL ${shellQuote(`${base}/install.sh`)} -o "$installer"`;
  const tokenSetup = options.includeEnrollmentToken
    ? `read -r -s -p 'Paste enrollment token: ' token <&3\nprintf '\\n'\nprintf '%s\\n' "$token" > "$token_file"\nunset token\ntoken_args=(--enrollment-token-file "$token_file")`
    : "token_args=()";
  const tokenArgsExpansion = "$" + "{token_args[@]}";
  return `bash 3<&0 <<'LABTETHER_INSTALL'\nset -euo pipefail\numask 077\ninstaller="$(mktemp)"\ntoken_file="$(mktemp)"\nca_file=""\ncleanup() {\n  rm -f "$installer" "$token_file"\n  if [[ -n "$ca_file" ]]; then rm -f "$ca_file"; fi\n}\ntrap cleanup EXIT\n${installerDownload}\nchmod 700 "$installer"\n${tokenSetup}\nsudo bash "$installer" "${tokenArgsExpansion}" \\\n  ${flags.join(" \\\n  ")}\nLABTETHER_INSTALL`;
}

const HUB_TARGET_KEYS = new Map([
  ["external", "external"],
  ["request", "request"],
  ["tailscale", "tailscale"],
  ["tailscale_https", "tailscaleHttps"],
  ["lan", "lan"],
  ["fallback", "fallback"],
]);

/** The name of one of the hub's addresses, by its kind; an unknown kind keeps the hub's own label. */
export function hubCandidateLabel(t: AddDeviceT, candidate: HubConnectionCandidate): string {
  // The hub labels a loopback request address "Localhost" and any other request address "Request host".
  if (candidate.kind === "request" && candidate.label === "Localhost") {
    return t("agent.target.localhost");
  }
  const key = HUB_TARGET_KEYS.get(candidate.kind);
  if (key) return t(`agent.target.${key}`);
  return candidate.label || t("agent.target.other");
}

export function formatHubCandidateOption(t: AddDeviceT, candidate: HubConnectionCandidate): string {
  const label = hubCandidateLabel(t, candidate);
  return candidate.host ? t("agent.targetOption", { label, host: candidate.host }) : label;
}

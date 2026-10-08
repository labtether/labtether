"use client";

import { type HubConnectionCandidate } from "../../hooks/useEnrollment";

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
  return raw.trim().replace(/\/+$/, "");
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
    const expectedPath = `${base.pathname.replace(/\/+$/, "")}/api/v1/agent/bootstrap.sh`;
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

export function manualInstallCommand(
  platform: Platform,
  wsURL: string,
): string {
  if (platform === "windows") {
    return `$tokenFile = Join-Path $env:TEMP "labtether-enrollment-token-$PID"\n$secureToken = Read-Host "Paste enrollment token" -AsSecureString\n$pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureToken)\ntry {\n  [IO.File]::WriteAllText($tokenFile, [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer))\n} finally {\n  [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)\n}\n$env:LABTETHER_ENROLLMENT_TOKEN_FILE=$tokenFile\n$env:LABTETHER_WS_URL=${powershellQuote(wsURL)}\ntry {\n  .\\labtether-agent.exe\n} finally {\n  Remove-Item -Force -ErrorAction SilentlyContinue $tokenFile\n  Remove-Item Env:LABTETHER_ENROLLMENT_TOKEN_FILE -ErrorAction SilentlyContinue\n}`;
  }
  return `bash <<'LABTETHER_AGENT'\nset -euo pipefail\numask 077\ntoken_file="$(mktemp)"\ntrap 'rm -f "$token_file"' EXIT\nread -r -s -p 'Paste enrollment token: ' token\nprintf '\\n'\nprintf '%s\\n' "$token" > "$token_file"\nunset token\nLABTETHER_ENROLLMENT_TOKEN_FILE="$token_file" \\\n  LABTETHER_WS_URL=${shellQuote(wsURL)} \\\n  ./labtether-agent\nLABTETHER_AGENT`;
}

export function trustModeLabel(
  candidate: HubConnectionCandidate | null | undefined,
): string {
  switch (candidate?.trust_mode) {
    case "public_tls":
      return "Public / Tailscale trusted TLS";
    case "custom_tls":
      return "Operator-managed TLS";
    case "labtether_ca":
      return "LabTether built-in CA";
    case "plain_http":
      return "Plain HTTP";
    default:
      return "Connection";
  }
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
  const flags: string[] = [
    `--docker-enabled ${options.dockerEnabled}`,
    `--files-root-mode ${options.filesRootMode}`,
    `--auto-update ${options.autoUpdateEnabled ? "true" : "false"}`,
  ];

  if (options.autoInstallVNC) {
    flags.push("--install-vnc-prereqs");
  }

  if (options.dockerEnabled !== "false") {
    const endpoint =
      options.dockerEndpoint.trim() ||
      defaultLinuxInstallOptions.dockerEndpoint;
    flags.push(`--docker-endpoint ${shellQuote(endpoint)}`);

    const boundedInterval = parseBoundedDecimalInt(
      options.dockerDiscoveryIntervalSec,
      5,
      3600,
      30,
    );
    flags.push(`--docker-discovery-interval ${boundedInterval}`);
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
    ? `read -r -s -p 'Paste enrollment token: ' token\nprintf '\\n'\nprintf '%s\\n' "$token" > "$token_file"\nunset token\ntoken_args=(--enrollment-token-file "$token_file")`
    : "token_args=()";
  const tokenArgsExpansion = "$" + "{token_args[@]}";
  return `bash <<'LABTETHER_INSTALL'\nset -euo pipefail\numask 077\ninstaller="$(mktemp)"\ntoken_file="$(mktemp)"\nca_file=""\ncleanup() {\n  rm -f "$installer" "$token_file"\n  if [[ -n "$ca_file" ]]; then rm -f "$ca_file"; fi\n}\ntrap cleanup EXIT\n${installerDownload}\nchmod 700 "$installer"\n${tokenSetup}\nsudo bash "$installer" "${tokenArgsExpansion}" \\\n  ${flags.join(" \\\n  ")}\nLABTETHER_INSTALL`;
}

export function formatHubCandidateOption(candidate: HubConnectionCandidate): string {
  const label =
    candidate.label ||
    (candidate.kind === "tailscale"
      ? "Tailscale"
      : candidate.kind === "lan"
        ? "LAN"
        : "Connection");
  return candidate.host ? `${label} (${candidate.host})` : label;
}

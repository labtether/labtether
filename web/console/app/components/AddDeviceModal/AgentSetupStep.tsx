"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Copy, Check, Loader2 } from "lucide-react";
import { Button } from "../ui/Button";
import { Input, Select } from "../ui/Input";
import { useEnrollment, type HubConnectionCandidate } from "../../hooks/useEnrollment";
import { useFastStatus } from "../../contexts/StatusContext";
import { useToast } from "../../contexts/ToastContext";
import type { AddDeviceAddedEvent } from "./types";

type Platform = "linux" | "macos" | "windows";
type LinuxDockerMode = "auto" | "true" | "false";
type FilesRootMode = "home" | "full";

type LinuxInstallOptions = {
  dockerEnabled: LinuxDockerMode;
  dockerEndpoint: string;
  dockerDiscoveryIntervalSec: string;
  filesRootMode: FilesRootMode;
  autoInstallVNC: boolean;
  autoUpdateEnabled: boolean;
  forceUpdate: boolean;
  includeEnrollmentToken: boolean;
};

type AgentSetupStepProps = {
  onBack: () => void;
  onClose: () => void;
  onAdded?: (event: AddDeviceAddedEvent) => void;
};

const defaultLinuxInstallOptions: LinuxInstallOptions = {
  dockerEnabled: "auto",
  dockerEndpoint: "/var/run/docker.sock",
  dockerDiscoveryIntervalSec: "30",
  filesRootMode: "home",
  autoInstallVNC: true,
  autoUpdateEnabled: true,
  forceUpdate: false,
  includeEnrollmentToken: true,
};

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, [text]);
  return (
    <button onClick={handleCopy} className="p-1 rounded hover:bg-[var(--hover)] transition-colors duration-150" title="Copy">
      {copied ? <Check size={14} className="text-[var(--ok)]" /> : <Copy size={14} className="text-[var(--muted)]" />}
    </button>
  );
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\"'\"'`)}'`;
}

function normalizeHubURL(raw: string): string {
  return raw.trim().replace(/\/+$/, "");
}

function manualInstallCommand(platform: Platform, token: string, wsURL: string): string {
  if (platform === "windows") {
    return `$env:LABTETHER_ENROLLMENT_TOKEN="${token}"\n$env:LABTETHER_WS_URL="${wsURL}"\n.\\labtether-agent.exe`;
  }
  return `LABTETHER_ENROLLMENT_TOKEN="${token}" \\\n  LABTETHER_WS_URL="${wsURL}" \\\n  ./labtether-agent`;
}

function trustModeLabel(candidate: HubConnectionCandidate | null | undefined): string {
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

function linuxInstallerCommand(hubURL: string, token: string, options: LinuxInstallOptions, candidate?: HubConnectionCandidate | null): string {
  const base = normalizeHubURL(hubURL);
  const bootstrapURL = candidate?.bootstrap_url?.trim() ?? "";
  const usePinnedBootstrap = candidate?.bootstrap_strategy === "pinned_ca_bootstrap" && bootstrapURL !== "";
  const scriptURL = usePinnedBootstrap ? bootstrapURL : `${base}/install.sh`;
  const flags: string[] = [
    `--docker-enabled ${options.dockerEnabled}`,
    `--files-root-mode ${options.filesRootMode}`,
    `--auto-update ${options.autoUpdateEnabled ? "true" : "false"}`,
  ];

  if (options.autoInstallVNC) {
    flags.push("--install-vnc-prereqs");
  }

  if (options.dockerEnabled !== "false") {
    const endpoint = options.dockerEndpoint.trim() || defaultLinuxInstallOptions.dockerEndpoint;
    flags.push(`--docker-endpoint ${shellQuote(endpoint)}`);

    const parsedInterval = Number.parseInt(options.dockerDiscoveryIntervalSec, 10);
    const boundedInterval = Number.isFinite(parsedInterval) && parsedInterval >= 5 && parsedInterval <= 3600
      ? parsedInterval
      : 30;
    flags.push(`--docker-discovery-interval ${boundedInterval}`);
  }

  if (options.includeEnrollmentToken && token.trim() !== "") {
    flags.push(`--enrollment-token ${shellQuote(token.trim())}`);
  }
  if (options.forceUpdate) {
    flags.push("--force-update");
  }

  const curlFlags = usePinnedBootstrap ? "-kfsSL" : "-fsSL";
  return `curl ${curlFlags} ${shellQuote(scriptURL)} | sudo bash -s -- \\\n  ${flags.join(" \\\n  ")}`;
}

function formatHubCandidateOption(candidate: HubConnectionCandidate): string {
  const label = candidate.label || (candidate.kind === "tailscale" ? "Tailscale" : candidate.kind === "lan" ? "LAN" : "Connection");
  return candidate.host ? `${label} (${candidate.host})` : label;
}

export function AgentSetupStep({ onBack, onClose, onAdded }: AgentSetupStepProps) {
  const { addToast } = useToast();
  const [platform, setPlatform] = useState<Platform>("linux");
  const [linuxOptions, setLinuxOptions] = useState<LinuxInstallOptions>(defaultLinuxInstallOptions);
  const [showAdvancedSettings, setShowAdvancedSettings] = useState(false);
  const {
    hubURL,
    wsURL,
    hubCandidates,
    enrollmentTokens,
    selectHubURL,
    newRawToken,
    newTokenID,
    generating,
    generateToken,
    clearNewToken,
    error,
  } = useEnrollment();
  const selectedCandidate = useMemo(
    () => hubCandidates.find((candidate) => candidate.hub_url === hubURL) ?? null,
    [hubCandidates, hubURL],
  );
  const status = useFastStatus();
  const initialAssetCount = useRef<number | null>(null);
  const [deviceDetected, setDeviceDetected] = useState(false);
  const detectionHandledRef = useRef(false);
  const autoCloseTimerRef = useRef<number | null>(null);

  const isLinux = platform === "linux";
  const installerCommand = useMemo(
    () => (hubURL ? linuxInstallerCommand(hubURL, newRawToken, linuxOptions, selectedCandidate) : ""),
    [hubURL, newRawToken, linuxOptions, selectedCandidate]
  );
  const fallbackCommand = useMemo(
    () => manualInstallCommand(platform, newRawToken, wsURL),
    [platform, newRawToken, wsURL]
  );

  // Capture baseline asset count once status has loaded
  useEffect(() => {
    if (initialAssetCount.current === null && status?.assets) {
      initialAssetCount.current = status.assets.length;
    }
  }, [status]);

  // Auto-generate a token on mount
  useEffect(() => {
    void generateToken("add-device-wizard", 24, 1);
    return () => clearNewToken();
  }, [clearNewToken, generateToken]);

  // Poll for new device
  useEffect(() => {
    if (!newRawToken || deviceDetected || initialAssetCount.current === null) return;
    const currentCount = status?.assets?.length ?? 0;
    const createdEnrollmentToken = newTokenID
      ? enrollmentTokens.find((token) => token.id === newTokenID)
      : null;
    const tokenConsumed = Boolean(createdEnrollmentToken && createdEnrollmentToken.use_count > 0);
    if (currentCount > initialAssetCount.current || tokenConsumed) {
      setDeviceDetected(true);
    }
  }, [enrollmentTokens, status, newRawToken, newTokenID, deviceDetected]);

  useEffect(() => {
    if (!deviceDetected || detectionHandledRef.current) return;
    detectionHandledRef.current = true;
    addToast("success", "Agent connected successfully.");
    onAdded?.({ source: "agent" });
    autoCloseTimerRef.current = window.setTimeout(() => {
      onClose();
    }, 1200);
  }, [deviceDetected, addToast, onClose, onAdded]);

  useEffect(() => {
    return () => {
      if (autoCloseTimerRef.current !== null) {
        window.clearTimeout(autoCloseTimerRef.current);
        autoCloseTimerRef.current = null;
      }
    };
  }, []);

  const platforms: { id: Platform; label: string }[] = [
    { id: "linux", label: "Linux" },
    { id: "macos", label: "macOS" },
    { id: "windows", label: "Windows" },
  ];

  if (deviceDetected) {
    return (
      <div className="flex flex-col items-center gap-4 py-8">
        <div className="w-10 h-10 rounded-full bg-[var(--ok-glow)] flex items-center justify-center">
          <Check size={20} className="text-[var(--ok)]" />
        </div>
        <p className="text-sm font-medium text-[var(--text)]">Device connected successfully</p>
        <Button variant="primary" onClick={onClose}>Done</Button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {error && <p className="text-xs text-[var(--bad)]">{error}</p>}

      {/* Platform picker */}
      <div>
        <p className="text-xs font-medium text-[var(--muted)] mb-2">Platform</p>
        <div className="flex gap-2">
          {platforms.map((p) => (
            <button
              key={p.id}
              onClick={() => setPlatform(p.id)}
              className={`px-3 py-1.5 text-xs rounded-lg border transition-colors duration-150 ${
                platform === p.id
                  ? "border-[var(--accent)] text-[var(--accent)] bg-[var(--accent)]/10"
                  : "border-[var(--line)] text-[var(--muted)] hover:bg-[var(--hover)]"
              }`}
            >
              {p.label}
            </button>
          ))}
        </div>
      </div>

      {generating ? (
        <div className="flex items-center gap-2 py-4 text-sm text-[var(--muted)]">
          <Loader2 size={16} className="animate-spin" /> Generating enrollment token...
        </div>
      ) : newRawToken ? (
        <>
          {/* Token */}
          <div>
            <p className="text-xs font-medium text-[var(--muted)] mb-1">Enrollment Token</p>
            <div className="flex items-center gap-2 bg-[var(--surface)] rounded-lg px-3 py-2">
              <code className="text-xs text-[var(--text)] flex-1 truncate">{newRawToken}</code>
              <CopyButton text={newRawToken} />
            </div>
          </div>

          {/* Hub info */}
          {hubCandidates.length > 1 ? (
            <div>
              <p className="text-xs font-medium text-[var(--muted)] mb-1">Connection Target</p>
              <Select value={hubURL} onChange={(event) => selectHubURL(event.target.value)}>
                {hubCandidates.map((candidate) => (
                  <option key={candidate.hub_url} value={candidate.hub_url}>
                    {formatHubCandidateOption(candidate)}
                  </option>
                ))}
              </Select>
            </div>
          ) : null}

          {selectedCandidate?.preferred_reason ? (
            <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-xs text-[var(--muted)]">
              <span className="font-medium text-[var(--text)]">{trustModeLabel(selectedCandidate)}.</span>{" "}
              {selectedCandidate.preferred_reason}
            </div>
          ) : null}

          <div className="grid grid-cols-2 gap-3">
            <div>
              <p className="text-xs font-medium text-[var(--muted)] mb-1">Hub URL</p>
              <div className="flex items-center gap-1 bg-[var(--surface)] rounded-lg px-3 py-2">
                <code className="text-xs text-[var(--text)] flex-1 truncate">{hubURL}</code>
                <CopyButton text={hubURL} />
              </div>
            </div>
            <div>
              <p className="text-xs font-medium text-[var(--muted)] mb-1">WebSocket URL</p>
              <div className="flex items-center gap-1 bg-[var(--surface)] rounded-lg px-3 py-2">
                <code className="text-xs text-[var(--text)] flex-1 truncate">{wsURL}</code>
                <CopyButton text={wsURL} />
              </div>
            </div>
          </div>

	          {isLinux ? (
	            <div className="space-y-3 rounded-lg border border-[var(--line)] p-3">
	              <div>
	                <p className="text-xs font-medium text-[var(--text)] mb-1">Linux Installer Script (Recommended)</p>
	                <p className="text-xs text-[var(--muted)]">Choose the normal access settings here, then run the generated one-line installer.</p>
                  {selectedCandidate?.bootstrap_strategy === "pinned_ca_bootstrap" ? (
                    <p className="mt-1 text-xs text-[var(--warn)]">
                      This target uses LabTether&apos;s built-in CA, so the generated command bootstraps trust before running the installer.
                    </p>
                  ) : selectedCandidate?.trust_mode === "custom_tls" ? (
                    <p className="mt-1 text-xs text-[var(--muted)]">
                      This target uses operator-managed TLS. Make sure the uploaded certificate chain is already trusted by the target machine.
                    </p>
                  ) : null}
	              </div>

              <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                  File access
                  <Select
                    value={linuxOptions.filesRootMode}
                    onChange={(event) => setLinuxOptions((current) => ({ ...current, filesRootMode: event.target.value as FilesRootMode }))}
                  >
                    <option value="home">Home-only access</option>
                    <option value="full">Full disk access</option>
                  </Select>
                </label>

                <label className="flex items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-xs text-[var(--muted)]">
                  <input
                    type="checkbox"
                    checked={linuxOptions.autoInstallVNC}
                    onChange={(event) => setLinuxOptions((current) => ({ ...current, autoInstallVNC: event.target.checked }))}
                  />
                  Enable desktop prerequisites for VNC/remote view
                </label>

                <label className="flex items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-xs text-[var(--muted)] md:col-span-2">
                  <input
                    type="checkbox"
                    checked={linuxOptions.autoUpdateEnabled}
                    onChange={(event) => setLinuxOptions((current) => ({ ...current, autoUpdateEnabled: event.target.checked }))}
                  />
                  Keep the agent updated automatically
                </label>
              </div>

              <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)]">
                <button
                  type="button"
                  onClick={() => setShowAdvancedSettings((current) => !current)}
                  className="flex w-full items-center justify-between px-3 py-2 text-left"
                >
                  <span>
                    <span className="block text-xs font-medium text-[var(--text)]">Advanced settings</span>
                    <span className="block text-[11px] text-[var(--muted)]">
                      Docker discovery, force update, manual token control, and fallback binary install.
                    </span>
                  </span>
                  <span className="text-xs text-[var(--muted)]">
                    {showAdvancedSettings ? "Hide" : "Show"}
                  </span>
                </button>

                {showAdvancedSettings ? (
                  <div className="grid grid-cols-1 gap-3 border-t border-[var(--line)] px-3 py-3 md:grid-cols-2">
                    <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                      Docker mode
                      <Select
                        value={linuxOptions.dockerEnabled}
                        onChange={(event) => setLinuxOptions((current) => ({ ...current, dockerEnabled: event.target.value as LinuxDockerMode }))}
                      >
                        <option value="auto">auto</option>
                        <option value="true">true</option>
                        <option value="false">false</option>
                      </Select>
                    </label>

                    <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                      Docker discovery interval (sec)
                      <Input
                        type="number"
                        min={5}
                        max={3600}
                        value={linuxOptions.dockerDiscoveryIntervalSec}
                        disabled={linuxOptions.dockerEnabled === "false"}
                        onChange={(event) => setLinuxOptions((current) => ({ ...current, dockerDiscoveryIntervalSec: event.target.value }))}
                      />
                    </label>

                    <label className="flex flex-col gap-1 text-xs text-[var(--muted)] md:col-span-2">
                      Docker endpoint
                      <Input
                        value={linuxOptions.dockerEndpoint}
                        disabled={linuxOptions.dockerEnabled === "false"}
                        onChange={(event) => setLinuxOptions((current) => ({ ...current, dockerEndpoint: event.target.value }))}
                        placeholder="/var/run/docker.sock"
                      />
                    </label>

                    <label className="flex items-center gap-2 text-xs text-[var(--muted)]">
                      <input
                        type="checkbox"
                        checked={linuxOptions.forceUpdate}
                        onChange={(event) => setLinuxOptions((current) => ({ ...current, forceUpdate: event.target.checked }))}
                      />
                      Force update immediately after install
                    </label>

                    <label className="flex items-center gap-2 text-xs text-[var(--muted)]">
                      <input
                        type="checkbox"
                        checked={linuxOptions.includeEnrollmentToken}
                        onChange={(event) => setLinuxOptions((current) => ({ ...current, includeEnrollmentToken: event.target.checked }))}
                      />
                      Include one-time enrollment token
                    </label>

                    <div className="md:col-span-2">
                      <p className="text-xs font-medium text-[var(--muted)] mb-1">Manual binary command</p>
                      <div className="relative bg-[var(--panel)] rounded-lg px-3 py-2">
                        <pre className="text-xs text-[var(--text)] whitespace-pre-wrap">{fallbackCommand}</pre>
                        <div className="absolute top-2 right-2">
                          <CopyButton text={fallbackCommand} />
                        </div>
                      </div>
                    </div>
                  </div>
                ) : null}
              </div>

              {!linuxOptions.includeEnrollmentToken ? (
                <p className="text-xs text-[var(--warn)]">
                  Token disabled: this install uses pending approval flow instead of auto-enrollment.
                </p>
              ) : null}

              <div>
                <p className="text-xs font-medium text-[var(--muted)] mb-1">Run on Linux target</p>
                <div className="relative bg-[var(--surface)] rounded-lg px-3 py-2">
                  <pre className="text-xs text-[var(--text)] whitespace-pre-wrap">{installerCommand}</pre>
                  <div className="absolute top-2 right-2">
                    <CopyButton text={installerCommand} />
                  </div>
                </div>
              </div>
            </div>
          ) : null}

          {/* Manual command */}
          {!isLinux ? (
            <div>
              <p className="text-xs font-medium text-[var(--muted)] mb-1">Run on target device</p>
              <div className="relative bg-[var(--surface)] rounded-lg px-3 py-2">
                <pre className="text-xs text-[var(--text)] whitespace-pre-wrap">{fallbackCommand}</pre>
                <div className="absolute top-2 right-2">
                  <CopyButton text={fallbackCommand} />
                </div>
              </div>
            </div>
          ) : null}

          {/* Waiting indicator */}
          <div className="flex items-center gap-2 text-xs text-[var(--muted)] pt-2">
            <Loader2 size={14} className="animate-spin" />
            Waiting for device to check in...
          </div>
        </>
      ) : null}

      <div className="flex items-center gap-3 pt-2">
        <Button onClick={onBack}>Back</Button>
      </div>
    </div>
  );
}

"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Check, Loader2 } from "lucide-react";
import { Button } from "../ui/Button";
import { Input, Select } from "../ui/Input";
import { deleteEnrollmentToken, useEnrollment } from "../../hooks/useEnrollment";
import { useFastStatus } from "../../contexts/StatusContext";
import { useToast } from "../../contexts/ToastContext";
import type { AddDeviceAddedEvent } from "./types";
import { type Platform, type LinuxInstallOptions, defaultLinuxInstallOptions, linuxInstallerCommand, manualInstallCommand, formatHubCandidateOption, trustModeLabel } from "./agentInstallCommands";
import { CopyButton } from "./EnrollmentCopyButton";
import { LinuxAgentInstallForm } from "./LinuxAgentInstallForm";
export type { LinuxInstallOptions } from "./agentInstallCommands";
export { manualInstallCommand } from "./agentInstallCommands";
export { linuxInstallerCommand } from "./agentInstallCommands";

type AgentSetupStepProps = {
  onBack: () => void;
  onClose: () => void;
  onAdded?: (event: AddDeviceAddedEvent) => void;
};

export function AgentSetupStep({
  onBack,
  onClose,
  onAdded,
}: AgentSetupStepProps) {
  const { addToast } = useToast();
  const [platform, setPlatform] = useState<Platform>("linux");
  const [linuxOptions, setLinuxOptions] = useState<LinuxInstallOptions>(
    defaultLinuxInstallOptions,
  );
  const [showAdvancedSettings, setShowAdvancedSettings] = useState(false);
  const [expectedHostname, setExpectedHostname] = useState("");
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
    () =>
      hubCandidates.find((candidate) => candidate.hub_url === hubURL) ?? null,
    [hubCandidates, hubURL],
  );
  const status = useFastStatus();
  const initialAssetCount = useRef<number | null>(null);
  const [deviceDetected, setDeviceDetected] = useState(false);
  const detectionHandledRef = useRef(false);
  const autoCloseTimerRef = useRef<number | null>(null);
  const mountedRef = useRef(true);
  const currentTokenIDRef = useRef("");
  const tokenConsumedRef = useRef(false);
  const tokenCopiedRef = useRef(false);
  const discardedTokenIDsRef = useRef(new Set<string>());

  const discardUntouchedToken = useCallback((tokenID: string) => {
    if (
      !tokenID ||
      tokenConsumedRef.current ||
      tokenCopiedRef.current ||
      discardedTokenIDsRef.current.has(tokenID)
    ) {
      return;
    }
    discardedTokenIDsRef.current.add(tokenID);
    void deleteEnrollmentToken(tokenID, { keepalive: true }).catch(() => {
      // This is best-effort teardown during navigation. The token remains
      // visible in Settings if the request cannot complete, so an operator can
      // revoke it explicitly rather than receiving a false cleanup message.
    });
  }, []);

  const isLinux = platform === "linux";
  const installerCommand = useMemo(
    () =>
      hubURL
        ? linuxInstallerCommand(hubURL, linuxOptions, selectedCandidate)
        : "",
    [hubURL, linuxOptions, selectedCandidate],
  );
  const fallbackCommand = useMemo(
    () => manualInstallCommand(platform, wsURL),
    [platform, wsURL],
  );

  // Capture baseline asset count once status has loaded
  useEffect(() => {
    if (initialAssetCount.current === null && status?.assets) {
      initialAssetCount.current = status.assets.length;
    }
  }, [status]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      discardUntouchedToken(currentTokenIDRef.current);
      clearNewToken();
    };
  }, [clearNewToken, discardUntouchedToken]);

  const createScopedToken = useCallback(() => {
    const hostname = expectedHostname.trim();
    if (!hostname) return;
    void generateToken("add-device-wizard", 24, 1, {
      scope: "asset",
      assetID: hostname,
    }).then((tokenID) => {
      currentTokenIDRef.current = tokenID;
      if (!mountedRef.current) {
        discardUntouchedToken(tokenID);
      }
    });
  }, [discardUntouchedToken, expectedHostname, generateToken]);

  useEffect(() => {
    currentTokenIDRef.current = newTokenID;
  }, [newTokenID]);

  // Poll for new device
  useEffect(() => {
    if (!newRawToken || deviceDetected || initialAssetCount.current === null)
      return;
    const currentCount = status?.assets?.length ?? 0;
    const createdEnrollmentToken = newTokenID
      ? enrollmentTokens.find((token) => token.id === newTokenID)
      : null;
    const tokenConsumed = Boolean(
      createdEnrollmentToken && createdEnrollmentToken.use_count > 0,
    );
    if (tokenConsumed) {
      tokenConsumedRef.current = true;
    }
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
        <p className="text-sm font-medium text-[var(--text)]">
          Device connected successfully
        </p>
        <Button variant="primary" onClick={onClose}>
          Done
        </Button>
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

      {!newRawToken && !generating ? (
        <div className="space-y-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
          <label className="flex flex-col gap-1 text-xs font-medium text-[var(--muted)]">
            Expected hostname
            <Input
              aria-label="Expected hostname"
              placeholder="example: server-01"
              value={expectedHostname}
              onChange={(event) => setExpectedHostname(event.target.value)}
            />
          </label>
          <p className="text-xs text-[var(--muted)]">
            This makes the one-time token work only for this device name.
          </p>
          <Button
            variant="primary"
            disabled={!expectedHostname.trim()}
            onClick={createScopedToken}
          >
            Create one-time token
          </Button>
        </div>
      ) : null}

      {generating ? (
        <div className="flex items-center gap-2 py-4 text-sm text-[var(--muted)]">
          <Loader2 size={16} className="animate-spin" /> Generating enrollment
          token...
        </div>
      ) : newRawToken ? (
        <>
          {/* Token */}
          <div>
            <p className="text-xs font-medium text-[var(--muted)] mb-1">
              Enrollment Token
            </p>
            <div className="flex items-center gap-2 bg-[var(--surface)] rounded-lg px-3 py-2">
              <code className="text-xs text-[var(--text)] flex-1 truncate">
                {newRawToken}
              </code>
              <CopyButton
                text={newRawToken}
                label="enrollment token"
                onCopy={() => {
                  tokenCopiedRef.current = true;
                }}
              />
            </div>
          </div>

          {/* Hub info */}
          {hubCandidates.length > 1 ? (
            <div>
              <p className="text-xs font-medium text-[var(--muted)] mb-1">
                Connection Target
              </p>
              <Select
                aria-label="Connection target"
                value={hubURL}
                onChange={(event) => selectHubURL(event.target.value)}
              >
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
              <span className="font-medium text-[var(--text)]">
                {trustModeLabel(selectedCandidate)}.
              </span>{" "}
              {selectedCandidate.preferred_reason}
            </div>
          ) : null}

          <div className="grid grid-cols-2 gap-3">
            <div>
              <p className="text-xs font-medium text-[var(--muted)] mb-1">
                Hub URL
              </p>
              <div className="flex items-center gap-1 bg-[var(--surface)] rounded-lg px-3 py-2">
                <code className="text-xs text-[var(--text)] flex-1 truncate">
                  {hubURL}
                </code>
                <CopyButton text={hubURL} label="Hub URL" />
              </div>
            </div>
            <div>
              <p className="text-xs font-medium text-[var(--muted)] mb-1">
                WebSocket URL
              </p>
              <div className="flex items-center gap-1 bg-[var(--surface)] rounded-lg px-3 py-2">
                <code className="text-xs text-[var(--text)] flex-1 truncate">
                  {wsURL}
                </code>
                <CopyButton text={wsURL} label="WebSocket URL" />
              </div>
            </div>
          </div>

          {isLinux ? (
            <LinuxAgentInstallForm
              selectedCandidate={selectedCandidate}
              linuxOptions={linuxOptions}
              setLinuxOptions={setLinuxOptions}
              showAdvancedSettings={showAdvancedSettings}
              setShowAdvancedSettings={setShowAdvancedSettings}
              fallbackCommand={fallbackCommand}
              installerCommand={installerCommand}
            />
          ) : null}

          {/* Manual command */}
          {!isLinux ? (
            <div>
              <p className="text-xs font-medium text-[var(--muted)] mb-1">
                Run on target device
              </p>
              <div className="relative bg-[var(--surface)] rounded-lg px-3 py-2">
                <pre className="text-xs text-[var(--text)] whitespace-pre-wrap">
                  {fallbackCommand}
                </pre>
                <div className="absolute top-2 right-2">
                  <CopyButton
                    text={fallbackCommand}
                    label={`${platform} agent command`}
                  />
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

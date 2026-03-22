"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "../ui/Button";
import { Input } from "../ui/Input";
import { useProxmoxSettings } from "../../hooks/useProxmoxSettings";
import { useToast } from "../../contexts/ToastContext";
import { useStatusControls } from "../../contexts/StatusContext";
import { baseURLHostLabel, validateHTTPSOrHTTPURL, validatePollIntervalSeconds, validateProxmoxTokenID } from "./validation";
import { monitorCollectorRunWithRetry } from "./collectorSync";
import type { AddDeviceAddedEvent, AddDeviceCompatPrefill, SetupMode } from "./types";

type ProxmoxSetupStepProps = {
  onBack: () => void;
  onClose: () => void;
  onAdded?: (event: AddDeviceAddedEvent) => void;
  compatPrefills?: AddDeviceCompatPrefill[];
  setupMode: SetupMode;
};

export function ProxmoxSetupStep({ onBack, onClose, onAdded, compatPrefills = [], setupMode }: ProxmoxSetupStepProps) {
  const { addToast } = useToast();
  const { fetchStatus } = useStatusControls();
  const {
    baseURL, setBaseURL,
    authMethod, setAuthMethod,
    tokenID, setTokenID,
    tokenSecret, setTokenSecret,
    username, setUsername,
    skipVerify, setSkipVerify,
    clusterName, setClusterName,
    intervalSeconds, setIntervalSeconds,
    collectorID,
    configured,
    loading,
    saving, testing, running,
    error, message,
    save, testConnection, runNow,
  } = useProxmoxSettings();

  const baseURLError = validateHTTPSOrHTTPURL(baseURL);
  const intervalError = validatePollIntervalSeconds(intervalSeconds);
  const tokenIDError = authMethod === "api_token" ? validateProxmoxTokenID(tokenID) : "";
  const usernameError = authMethod === "password" && !username.trim() ? "Username is required." : "";

  const saveValidationError = [baseURLError, intervalError, tokenIDError, usernameError].find((entry) => entry) ?? "";
  const testValidationError = [baseURLError, tokenIDError, usernameError].find((entry) => entry) ?? "";

  const handleSave = async () => {
    if (saveValidationError) {
      setFormError(saveValidationError);
      addToast("error", saveValidationError);
      return;
    }
    setFormError("");

    const result = await save();
    if (!result.ok) {
      addToast("error", result.error || "Failed to save Proxmox settings.");
      return;
    }

    if (result.warning) {
      addToast("warning", result.warning);
    }

    if (result.collectorID) {
      monitorCollectorRunWithRetry(result.collectorID, "Proxmox", addToast);
    }

    const focusQuery = clusterName.trim() || baseURLHostLabel(baseURL);
    onAdded?.({ source: "proxmox", focusQuery });
    addToast("success", "Proxmox connector saved.");
    void fetchStatus();
    onClose();
  };

  const handleTestConnection = async () => {
    if (testValidationError) {
      setFormError(testValidationError);
      addToast("error", testValidationError);
      return;
    }
    setFormError("");
    await testConnection();
  };

  const [formError, setFormError] = useState("");
  const prefillAppliedRef = useRef(false);
  const [selectedCompatBaseURL, setSelectedCompatBaseURL] = useState("");
  const selectedCompat = compatPrefills.find((item) => item.baseURL === selectedCompatBaseURL) ?? compatPrefills[0];

  const applyCompatPrefill = useCallback((prefill: AddDeviceCompatPrefill, forceName: boolean) => {
    setBaseURL(prefill.baseURL);
    if (forceName || !clusterName.trim()) {
      setClusterName(prefill.serviceName || "Homelab Proxmox");
    }
  }, [clusterName, setBaseURL, setClusterName]);

  useEffect(() => {
    if (compatPrefills.length === 0) {
      setSelectedCompatBaseURL("");
      return;
    }
    if (!selectedCompatBaseURL) {
      setSelectedCompatBaseURL(compatPrefills[0].baseURL);
    }
  }, [compatPrefills, selectedCompatBaseURL]);

  useEffect(() => {
    if (prefillAppliedRef.current) return;
    if (loading) return;
    if (!selectedCompat) return;
    if (configured) {
      prefillAppliedRef.current = true;
      return;
    }

    if (!baseURL.trim() || !clusterName.trim()) {
      applyCompatPrefill(selectedCompat, false);
    }
    prefillAppliedRef.current = true;
  }, [applyCompatPrefill, loading, configured, selectedCompat, baseURL, clusterName]);

  return (
    <div className="space-y-3">
      {error && <p className="text-xs text-[var(--bad)]">{error}</p>}
      {message && <p className="text-xs text-[var(--ok)]">{message}</p>}
      {formError ? <p className="text-xs text-[var(--warn)]">{formError}</p> : null}

      {compatPrefills.length > 1 && (
        <div>
          <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Detected Endpoint</label>
          <select
            value={selectedCompatBaseURL || compatPrefills[0].baseURL}
            onChange={(event) => {
              const nextBaseURL = event.target.value;
              setSelectedCompatBaseURL(nextBaseURL);
              const next = compatPrefills.find((item) => item.baseURL === nextBaseURL);
              if (next) {
                applyCompatPrefill(next, true);
              }
            }}
            className="w-full rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-2 text-xs text-[var(--text)]"
          >
            {compatPrefills.map((prefill) => (
              <option key={prefill.baseURL} value={prefill.baseURL}>
                {formatCompatPrefillOption(prefill)}
              </option>
            ))}
          </select>
          <p className="mt-1 text-xs text-[var(--muted)]">Selecting a detected endpoint updates base URL and cluster name.</p>
        </div>
      )}

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Base URL</label>
        <Input value={baseURL} onChange={(e) => setBaseURL(e.target.value)} placeholder="https://pve.local:8006" />
      </div>

      <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2">
        <p className="text-xs font-medium text-[var(--text)]">Credential Template</p>
        <p className="text-xs text-[var(--muted)]">
          Recommended API token format: <code className="text-[var(--text)]">labtether@pve!monitoring</code> with read-only inventory
          and metrics permissions for initial setup.
        </p>
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1.5 block">Authentication Method</label>
        <div className="flex gap-2">
          <AuthMethodButton
            active={authMethod === "api_token"}
            onClick={() => setAuthMethod("api_token")}
            label="API Token"
          />
          <AuthMethodButton
            active={authMethod === "password"}
            onClick={() => setAuthMethod("password")}
            label="Username & Password"
          />
        </div>
      </div>

      {authMethod === "api_token" ? (
        <>
          <div className="rounded-lg border border-[var(--warn)]/30 bg-[var(--warn-glow)] px-3 py-2">
            <p className="text-xs text-[var(--warn)]">
              API tokens do not support Proxmox console/terminal access due to a known Proxmox limitation (bug #6079). Terminal sessions will fall back to SSH.
            </p>
          </div>
          <div>
            <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Token ID</label>
            <Input value={tokenID} onChange={(e) => setTokenID(e.target.value)} placeholder="labtether@pve!monitoring" />
          </div>
          <div>
            <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Token Secret</label>
            <Input
              type="password"
              value={tokenSecret}
              onChange={(e) => setTokenSecret(e.target.value)}
              placeholder={configured ? "•••••••• (unchanged)" : "Required for initial setup"}
            />
          </div>
        </>
      ) : (
        <>
          <div>
            <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Username</label>
            <Input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="root@pam" />
          </div>
          <div>
            <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Password</label>
            <Input
              type="password"
              value={tokenSecret}
              onChange={(e) => setTokenSecret(e.target.value)}
              placeholder={configured ? "•••••••• (unchanged)" : "Required for initial setup"}
            />
          </div>
        </>
      )}

      {setupMode === "advanced" ? (
        <>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Cluster Name</label>
              <Input value={clusterName} onChange={(e) => setClusterName(e.target.value)} placeholder="Homelab Proxmox" />
            </div>
            <div>
              <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Poll Interval (s)</label>
              <Input type="number" min={15} max={3600} value={intervalSeconds} onChange={(e) => setIntervalSeconds(Number(e.target.value) || 60)} />
            </div>
          </div>

          <label className="flex items-center gap-2 text-xs text-[var(--muted)]">
            <input type="checkbox" checked={skipVerify} onChange={(e) => setSkipVerify(e.target.checked)} />
            Skip TLS certificate verification
          </label>
        </>
      ) : (
        <div>
          <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Cluster Name</label>
          <Input value={clusterName} onChange={(e) => setClusterName(e.target.value)} placeholder="Homelab Proxmox" />
          <p className="mt-1 text-xs text-[var(--muted)]">Default setup uses 60s polling with TLS verification enabled.</p>
        </div>
      )}

      <div className="flex items-center gap-3 pt-2">
        <Button onClick={onBack}>Back</Button>
        <Button disabled={testing || saving || running} onClick={() => void handleTestConnection()}>
          {testing ? "Testing..." : "Test Connection"}
        </Button>
        <Button disabled={!collectorID || saving || testing || running} onClick={() => void runNow()}>
          {running ? "Starting..." : "Run Sync"}
        </Button>
        <Button variant="primary" disabled={saving || testing || running || Boolean(saveValidationError)} onClick={() => void handleSave()}>
          {saving ? "Saving..." : "Save, Sync & Close"}
        </Button>
      </div>
    </div>
  );
}

function AuthMethodButton({ active, onClick, label }: { active: boolean; onClick: () => void; label: string }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`px-3 py-1.5 text-xs font-medium rounded-md border transition-colors duration-150 ${
        active
          ? "border-[var(--accent)] bg-[var(--accent)]/10 text-[var(--accent)]"
          : "border-[var(--line)] bg-transparent text-[var(--muted)] hover:border-[var(--muted)]"
      }`}
    >
      {label}
    </button>
  );
}

function formatCompatPrefillOption(prefill: AddDeviceCompatPrefill): string {
  const pct = `${Math.round(Math.max(0, Math.min(1, prefill.confidence)) * 100)}%`;
  const label = prefill.serviceName || prefill.baseURL;
  return `${label} (${prefill.baseURL}, ${pct})`;
}

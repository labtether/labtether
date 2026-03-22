"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Loader2 } from "lucide-react";
import { Button } from "../ui/Button";
import { Input } from "../ui/Input";
import { useTrueNASSettings } from "../../hooks/useTrueNASSettings";
import { useToast } from "../../contexts/ToastContext";
import { useStatusControls } from "../../contexts/StatusContext";
import { baseURLHostLabel, validateHTTPSOrHTTPURL, validatePollIntervalSeconds } from "./validation";
import { monitorCollectorRunWithRetry } from "./collectorSync";
import type { AddDeviceAddedEvent, AddDeviceCompatPrefill, SetupMode } from "./types";

type TrueNASSetupStepProps = {
  onBack: () => void;
  onClose: () => void;
  onAdded?: (event: AddDeviceAddedEvent) => void;
  compatPrefills?: AddDeviceCompatPrefill[];
  setupMode: SetupMode;
};

export function TrueNASSetupStep({ onBack, onClose, onAdded, compatPrefills = [], setupMode }: TrueNASSetupStepProps) {
  const { addToast } = useToast();
  const { fetchStatus } = useStatusControls();
  const {
    baseURL,
    setBaseURL,
    apiKey,
    setApiKey,
    displayName,
    setDisplayName,
    intervalSeconds,
    setIntervalSeconds,
    skipVerify,
    setSkipVerify,
    collectorID,
    configured,
    loading,
    saving,
    testing,
    running,
    error,
    message,
    testConnection,
    save,
    runNow,
  } = useTrueNASSettings();

  const [formError, setFormError] = useState("");
  const prefillAppliedRef = useRef(false);
  const [selectedCompatBaseURL, setSelectedCompatBaseURL] = useState("");
  const selectedCompat = compatPrefills.find((item) => item.baseURL === selectedCompatBaseURL) ?? compatPrefills[0];

  const applyCompatPrefill = useCallback((prefill: AddDeviceCompatPrefill, forceName: boolean) => {
    setBaseURL(prefill.baseURL);
    if (forceName || !displayName.trim()) {
      setDisplayName(prefill.serviceName || "Homelab TrueNAS");
    }
  }, [displayName, setBaseURL, setDisplayName]);

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

    if (!baseURL.trim() || !displayName.trim()) {
      applyCompatPrefill(selectedCompat, false);
    }
    prefillAppliedRef.current = true;
  }, [applyCompatPrefill, loading, configured, selectedCompat, baseURL, displayName]);

  const baseURLError = validateHTTPSOrHTTPURL(baseURL);
  const intervalError = validatePollIntervalSeconds(intervalSeconds);
  const apiKeyError = !apiKey.trim() ? "API Key is required." : "";
  const saveValidationError = [baseURLError, intervalError, apiKeyError].find((entry) => entry) ?? "";
  const testValidationError = [baseURLError, apiKeyError].find((entry) => entry) ?? "";

  const handleSave = async () => {
    if (saveValidationError) {
      setFormError(saveValidationError);
      addToast("error", saveValidationError);
      return;
    }
    setFormError("");

    const result = await save();
    if (!result.ok) {
      addToast("error", result.error || "Failed to save TrueNAS settings.");
      return;
    }

    if (result.warning) {
      addToast("warning", result.warning);
    }

    if (result.collectorID) {
      monitorCollectorRunWithRetry(result.collectorID, "TrueNAS", addToast);
    }

    const focusQuery = displayName.trim() || baseURLHostLabel(baseURL);
    onAdded?.({ source: "truenas", focusQuery });
    addToast("success", "TrueNAS connector saved.");
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

  if (loading) {
    return <p className="text-xs text-[var(--muted)]">Loading TrueNAS settings...</p>;
  }

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
          <p className="mt-1 text-xs text-[var(--muted)]">Selecting a detected endpoint updates base URL and display name.</p>
        </div>
      )}

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Base URL</label>
        <Input value={baseURL} onChange={(e) => setBaseURL(e.target.value)} placeholder="https://truenas.local" />
      </div>

      <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2">
        <p className="text-xs font-medium text-[var(--text)]">Credential Template</p>
        <p className="text-xs text-[var(--muted)]">
          Create a dedicated TrueNAS API key for LabTether with read permissions for datasets, pools, SMART, and system health.
        </p>
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">API Key</label>
        <Input
          type="password"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          placeholder="Required"
        />
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Display Name</label>
        <Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder="Homelab TrueNAS" />
      </div>

      {setupMode === "advanced" ? (
        <>
          <div>
            <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Poll Interval (s)</label>
            <Input
              type="number"
              min={15}
              max={3600}
              value={intervalSeconds}
              onChange={(e) => setIntervalSeconds(Number(e.target.value) || 60)}
            />
          </div>

          <label className="flex items-center gap-2 text-xs text-[var(--muted)]">
            <input type="checkbox" checked={skipVerify} onChange={(e) => setSkipVerify(e.target.checked)} />
            Skip TLS certificate verification
          </label>
        </>
      ) : (
        <p className="text-xs text-[var(--muted)]">Default setup uses 60s polling with TLS verification enabled.</p>
      )}

      <div className="flex items-center gap-3 pt-2">
        <Button onClick={onBack}>Back</Button>
        <Button disabled={testing || saving || running} onClick={() => void handleTestConnection()}>
          {testing ? <Loader2 size={14} className="animate-spin mr-1 inline" /> : null}
          {testing ? "Testing..." : "Test Connection"}
        </Button>
        <Button disabled={!collectorID || saving || testing || running} onClick={() => void runNow()}>
          {running ? <Loader2 size={14} className="animate-spin mr-1 inline" /> : null}
          {running ? "Starting..." : "Run Sync"}
        </Button>
        <Button variant="primary" disabled={saving || testing || running || Boolean(saveValidationError)} onClick={() => void handleSave()}>
          {saving ? <Loader2 size={14} className="animate-spin mr-1 inline" /> : null}
          {saving ? "Saving..." : "Save, Sync & Close"}
        </Button>
      </div>
    </div>
  );
}

function formatCompatPrefillOption(prefill: AddDeviceCompatPrefill): string {
  const pct = `${Math.round(Math.max(0, Math.min(1, prefill.confidence)) * 100)}%`;
  const label = prefill.serviceName || prefill.baseURL;
  return `${label} (${prefill.baseURL}, ${pct})`;
}

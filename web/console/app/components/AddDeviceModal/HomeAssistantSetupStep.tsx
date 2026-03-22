"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Check, Copy, ExternalLink, Loader2 } from "lucide-react";
import { Button } from "../ui/Button";
import { Input } from "../ui/Input";
import { useToast } from "../../contexts/ToastContext";
import { useStatusControls } from "../../contexts/StatusContext";
import { useHomeAssistantSettings } from "../../hooks/useHomeAssistantSettings";
import { baseURLHostLabel, validateHTTPSOrHTTPURL, validatePollIntervalSeconds } from "./validation";
import { monitorCollectorRunWithRetry } from "./collectorSync";
import type { AddDeviceAddedEvent, AddDeviceCompatPrefill, SetupMode } from "./types";

type HomeAssistantSetupStepProps = {
  onBack: () => void;
  onClose: () => void;
  onAdded?: (event: AddDeviceAddedEvent) => void;
  compatPrefills?: AddDeviceCompatPrefill[];
  setupMode: SetupMode;
};

const CUSTOM_INTEGRATION_DOC_URL = "https://github.com/labtether/labtether/blob/main/integrations/homeassistant/README.md";
const ADDON_DOC_URL = "https://github.com/labtether/labtether/blob/main/docs/HOME_ASSISTANT_ADDON.md";

type CopyRowProps = {
  label: string;
  value: string;
};

function CopyRow({ label, value }: CopyRowProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(value);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  }, [value]);

  return (
    <div className="space-y-1">
      <p className="text-xs font-medium text-[var(--muted)]">{label}</p>
      <div className="flex items-center gap-2 rounded-lg bg-[var(--surface)] px-3 py-2">
        <code className="flex-1 truncate text-xs text-[var(--text)]">{value}</code>
        <button
          type="button"
          onClick={handleCopy}
          className="rounded p-1 transition-colors duration-150 hover:bg-[var(--hover)]"
          title={`Copy ${label}`}
        >
          {copied ? <Check size={14} className="text-[var(--ok)]" /> : <Copy size={14} className="text-[var(--muted)]" />}
        </button>
      </div>
    </div>
  );
}

function DocsLink({ href, label }: { href: string; label: string }) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noreferrer"
      className="inline-flex items-center gap-1 rounded-lg border border-[var(--line)] px-3 py-1.5 text-xs font-medium text-[var(--text)] transition-colors duration-150 hover:bg-[var(--hover)]"
    >
      {label}
      <ExternalLink size={12} className="text-[var(--muted)]" />
    </a>
  );
}

export function HomeAssistantSetupStep({ onBack, onClose, onAdded, compatPrefills = [], setupMode }: HomeAssistantSetupStepProps) {
  const { addToast } = useToast();
  const { fetchStatus } = useStatusControls();
  const {
    baseURL,
    setBaseURL,
    token,
    setToken,
    displayName,
    setDisplayName,
    skipVerify,
    setSkipVerify,
    intervalSeconds,
    setIntervalSeconds,
    collectorID,
    credentialID,
    configured,
    loading,
    saving,
    testing,
    running,
    error,
    message,
    save,
    testConnection,
    runNow,
  } = useHomeAssistantSettings();

  const [formError, setFormError] = useState("");
  const prefillAppliedRef = useRef(false);
  const [selectedCompatBaseURL, setSelectedCompatBaseURL] = useState("");
  const selectedCompat = compatPrefills.find((item) => item.baseURL === selectedCompatBaseURL) ?? compatPrefills[0];

  const applyCompatPrefill = useCallback((prefill: AddDeviceCompatPrefill, forceName: boolean) => {
    setBaseURL(prefill.baseURL);
    if (forceName || !displayName.trim()) {
      setDisplayName(prefill.serviceName || "Home Assistant");
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

  const fallbackHubURL = useMemo(() => {
    if (typeof window === "undefined") {
      return "http://<labtether-host>:8080";
    }
    const protocol = window.location.protocol === "https:" ? "https:" : "http:";
    return `${protocol}//${window.location.hostname}:8080`;
  }, []);

  const baseURLError = validateHTTPSOrHTTPURL(baseURL);
  const intervalError = validatePollIntervalSeconds(intervalSeconds);
  const tokenRequiredError = (configured || Boolean(credentialID) || token.trim())
    ? ""
    : "Token is required for initial setup.";
  const saveValidationError = [baseURLError, intervalError, tokenRequiredError].find((entry) => entry) ?? "";
  const testValidationError = [baseURLError, (!token.trim() && !credentialID) ? "Token or saved credential is required for testing." : ""]
    .find((entry) => entry) ?? "";

  const handleSave = async () => {
    if (saveValidationError) {
      setFormError(saveValidationError);
      addToast("error", saveValidationError);
      return;
    }
    setFormError("");

    const result = await save();
    if (!result.ok) {
      addToast("error", result.error || "Failed to save Home Assistant settings.");
      return;
    }

    if (result.warning) {
      addToast("warning", result.warning);
    }

    if (result.collectorID) {
      monitorCollectorRunWithRetry(result.collectorID, "Home Assistant", addToast);
    }

    const focusQuery = displayName.trim() || baseURLHostLabel(baseURL);
    onAdded?.({ source: "homeassistant", focusQuery });
    addToast("success", "Home Assistant connector saved.");
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
    return <p className="text-xs text-[var(--muted)]">Loading Home Assistant settings...</p>;
  }

  return (
    <div className="space-y-4">
      {error && <p className="text-xs text-[var(--bad)]">{error}</p>}
      {message && <p className="text-xs text-[var(--ok)]">{message}</p>}
      {formError ? <p className="text-xs text-[var(--warn)]">{formError}</p> : null}

      {compatPrefills.length > 1 && (
        <div>
          <label className="mb-1 block text-xs font-medium text-[var(--muted)]">Detected Endpoint</label>
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

      <p className="text-xs text-[var(--muted)]">
        Home Assistant onboarding has two paths: install the custom integration now, or use the add-on packaging scaffold path.
      </p>

      <div className="space-y-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
        <div className="flex items-center justify-between gap-2">
          <p className="text-sm font-medium text-[var(--text)]">Custom Integration</p>
          <span className="rounded-full bg-[var(--ok-glow)] px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-[var(--ok)]">
            Available
          </span>
        </div>
        <p className="text-xs text-[var(--muted)]">
          Add the `labtether` custom integration in Home Assistant, then configure it with your LabTether hub URL and owner token.
        </p>
        <DocsLink href={CUSTOM_INTEGRATION_DOC_URL} label="Open Integration Setup Guide" />
      </div>

      <div className="space-y-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
        <div className="flex items-center justify-between gap-2">
          <p className="text-sm font-medium text-[var(--text)]">Home Assistant Add-on</p>
          <span className="rounded-full bg-[var(--warn-glow)] px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-[var(--warn)]">
            Scaffold
          </span>
        </div>
        <p className="text-xs text-[var(--muted)]">
          The install guide includes scaffold status and the eventual add-on install steps once repository publishing is available.
        </p>
        <DocsLink href={ADDON_DOC_URL} label="Open Add-on Install Guide" />
      </div>

      <div className="space-y-2 rounded-lg border border-[var(--line)] p-3">
        <p className="text-xs font-medium text-[var(--muted)]">Quick Config Values</p>
        <CopyRow label="Hub URL (example)" value={baseURL || fallbackHubURL} />
        <CopyRow label="Token variable" value="LABTETHER_OWNER_TOKEN" />
      </div>

      <div>
        <label className="mb-1 block text-xs font-medium text-[var(--muted)]">Base URL</label>
        <Input value={baseURL} onChange={(event) => setBaseURL(event.target.value)} placeholder="http://homeassistant.local:8123" />
      </div>

      <div>
        <label className="mb-1 block text-xs font-medium text-[var(--muted)]">Long-Lived Access Token</label>
        <Input
          type="password"
          value={token}
          onChange={(event) => setToken(event.target.value)}
          placeholder={configured ? "•••••••• (unchanged)" : "Required for initial setup"}
        />
      </div>

      <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2">
        <p className="text-xs font-medium text-[var(--text)]">Credential Template</p>
        <p className="text-xs text-[var(--muted)]">
          In Home Assistant, create a long-lived access token dedicated to LabTether. Name suggestion:
          <code className="text-[var(--text)]"> labtether_connector </code>.
        </p>
      </div>

      <div>
        <label className="mb-1 block text-xs font-medium text-[var(--muted)]">Display Name</label>
        <Input value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder="Home Assistant" />
      </div>

      {setupMode === "advanced" ? (
        <>
          <div>
            <label className="mb-1 block text-xs font-medium text-[var(--muted)]">Poll Interval (s)</label>
            <Input
              type="number"
              min={15}
              max={3600}
              value={intervalSeconds}
              onChange={(event) => setIntervalSeconds(Number(event.target.value) || 60)}
            />
          </div>

          <label className="flex items-center gap-2 text-xs text-[var(--muted)]">
            <input type="checkbox" checked={skipVerify} onChange={(event) => setSkipVerify(event.target.checked)} />
            Skip TLS certificate verification
          </label>
        </>
      ) : (
        <p className="text-xs text-[var(--muted)]">Default setup uses 60s polling with TLS verification enabled.</p>
      )}

      <div className="flex items-center gap-3 pt-1">
        <Button onClick={onBack}>Back</Button>
        <Button disabled={testing || saving || running} onClick={() => void handleTestConnection()}>
          {testing ? <Loader2 size={14} className="mr-1 inline animate-spin" /> : null}
          {testing ? "Testing..." : "Test Connection"}
        </Button>
        <Button disabled={!collectorID || saving || testing || running} onClick={() => void runNow()}>
          {running ? <Loader2 size={14} className="mr-1 inline animate-spin" /> : null}
          {running ? "Starting..." : "Run Sync"}
        </Button>
        <Button variant="primary" disabled={saving || testing || running || Boolean(saveValidationError)} onClick={() => void handleSave()}>
          {saving ? <Loader2 size={14} className="mr-1 inline animate-spin" /> : null}
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

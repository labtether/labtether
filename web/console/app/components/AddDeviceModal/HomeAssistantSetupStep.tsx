"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Check, Copy, ExternalLink, Loader2 } from "lucide-react";
import { Button } from "../ui/Button";
import { Input } from "../ui/Input";
import { useToast } from "../../contexts/ToastContext";
import { useStatusControls } from "../../contexts/StatusContext";
import { useHomeAssistantSettings } from "../../hooks/useHomeAssistantSettings";
import { useEnrollment, type HubConnectionCandidate } from "../../hooks/useEnrollment";
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

const CUSTOM_INTEGRATION_DOC_URL = "https://github.com/labtether/labtether-homeassistant/blob/main/README.md";
const ADDON_DOC_URL = "https://github.com/labtether/labtether-homeassistant/blob/main/addon/labtether/README.md";
const HOME_ASSISTANT_HUB_KINDS = new Set(["external", "tailscale_https", "tailscale"]);

function normalizedOrigin(rawURL: string): string {
  try {
    const parsed = new URL(rawURL);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") return "";
    if (parsed.username || parsed.password || parsed.pathname !== "/" || parsed.search || parsed.hash) return "";
    return parsed.origin;
  } catch {
    return "";
  }
}

export function homeAssistantHubCandidates(
  candidates: HubConnectionCandidate[],
  consoleOrigin: string,
): HubConnectionCandidate[] {
  const normalizedConsoleOrigin = normalizedOrigin(consoleOrigin);
  return candidates.filter((candidate) => {
    if (!HOME_ASSISTANT_HUB_KINDS.has(candidate.kind)) return false;
    const candidateOrigin = normalizedOrigin(candidate.hub_url);
    return candidateOrigin !== "" && candidateOrigin !== normalizedConsoleOrigin;
  });
}

export function preferredHomeAssistantHubURL(
  candidates: HubConnectionCandidate[],
  selectedHubURL: string,
  consoleOrigin: string,
): string {
  const eligibleCandidates = homeAssistantHubCandidates(candidates, consoleOrigin);
  const selectedCandidate = eligibleCandidates.find(
    (candidate) => candidate.hub_url === selectedHubURL
      && !validateHomeAssistantHubOrigin(candidate.hub_url)
      && !homeAssistantHubHTTPWarning(candidate.hub_url),
  );
  if (selectedCandidate) return selectedCandidate.hub_url;
  return eligibleCandidates.find(
    (candidate) => !validateHomeAssistantHubOrigin(candidate.hub_url)
      && !homeAssistantHubHTTPWarning(candidate.hub_url),
  )?.hub_url ?? "";
}

function normalizedHostname(rawHostname: string): string {
  return rawHostname.trim().toLowerCase().replace(/^\[|\]$/g, "").replace(/\.$/, "");
}

function isLoopbackHostname(rawHostname: string): boolean {
  const hostname = normalizedHostname(rawHostname);
  if (hostname === "localhost" || hostname === "::1") return true;
  const octets = hostname.split(".");
  return octets.length === 4
    && octets.every((octet) => /^\d{1,3}$/.test(octet) && Number(octet) <= 255)
    && Number(octets[0]) === 127;
}

export function validateHomeAssistantHubOrigin(rawURL: string): string {
  const value = rawURL.trim();
  if (!value) return "Enter a direct LabTether Hub URL that Home Assistant can reach.";
  if ([...value].some((character) => {
    const codePoint = character.codePointAt(0) ?? 0;
    return codePoint < 0x20 || codePoint === 0x7f;
  })) {
    return "LabTether Hub Address contains invalid control characters.";
  }

  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    return "LabTether Hub Address must be a valid URL.";
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    return "LabTether Hub Address must start with http:// or https://.";
  }
  if (!parsed.hostname || parsed.username || parsed.password) {
    return "LabTether Hub Address must be a credential-free origin.";
  }
  if (parsed.pathname !== "/" || parsed.search || parsed.hash) {
    return "LabTether Hub Address must not include a path, query, or fragment.";
  }
  if (parsed.port === "0") {
    return "LabTether Hub Address must use a valid port.";
  }
  return "";
}

export function homeAssistantHubHTTPWarning(rawURL: string): string {
  if (validateHomeAssistantHubOrigin(rawURL)) return "";
  const parsed = new URL(rawURL.trim());
  if (parsed.protocol === "http:" && !isLoopbackHostname(parsed.hostname)) {
    return "Plain HTTP requires explicitly enabling Allow insecure HTTP in Home Assistant.";
  }
  return "";
}

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
          aria-label={`Copy ${label}`}
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
  const {
    hubURL: selectedHubURL,
    hubCandidates,
  } = useEnrollment();

  const [formError, setFormError] = useState("");
  const [integrationHubURL, setIntegrationHubURL] = useState("");
  const integrationHubURLTouchedRef = useRef(false);
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

  const consoleOrigin = useMemo(() => {
    if (typeof window === "undefined") {
      return "";
    }
    return window.location.origin;
  }, []);
  const integrationHubCandidates = useMemo(
    () => homeAssistantHubCandidates(hubCandidates, consoleOrigin),
    [hubCandidates, consoleOrigin],
  );

  useEffect(() => {
    if (integrationHubURLTouchedRef.current) return;
    setIntegrationHubURL(preferredHomeAssistantHubURL(hubCandidates, selectedHubURL, consoleOrigin));
  }, [hubCandidates, selectedHubURL, consoleOrigin]);

  const integrationHubURLError = validateHomeAssistantHubOrigin(integrationHubURL);
  const integrationHubURLWarning = homeAssistantHubHTTPWarning(integrationHubURL);

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
      {error && <p role="alert" className="text-xs text-[var(--bad)]">{error}</p>}
      {message && <p role="status" aria-live="polite" className="text-xs text-[var(--ok)]">{message}</p>}
      {formError ? <p role="alert" className="text-xs text-[var(--warn)]">{formError}</p> : null}

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
        <div className="space-y-1">
          <label htmlFor="homeassistant-hub-url" className="block text-xs font-medium text-[var(--muted)]">
            LabTether Hub Address
          </label>
          <Input
            id="homeassistant-hub-url"
            list={integrationHubCandidates.length > 0 ? "homeassistant-hub-candidates" : undefined}
            value={integrationHubURL}
            onChange={(event) => {
              integrationHubURLTouchedRef.current = true;
              setIntegrationHubURL(event.target.value);
            }}
            placeholder="https://labtether.example:8443"
            aria-describedby="homeassistant-hub-url-help"
          />
          {integrationHubCandidates.length > 0 ? (
            <datalist id="homeassistant-hub-candidates">
              {integrationHubCandidates.map((candidate) => (
                <option key={candidate.hub_url} value={candidate.hub_url}>
                  {candidate.label || candidate.kind}
                </option>
              ))}
            </datalist>
          ) : null}
          <p id="homeassistant-hub-url-help" className="text-xs text-[var(--muted)]">
            Use the direct Hub address, not this web console address. Configured external and Tailscale addresses are suggested; enter a reachable LAN address manually when needed.
          </p>
          {integrationHubURLError ? <p role="alert" className="text-xs text-[var(--warn)]">{integrationHubURLError}</p> : null}
          {integrationHubURLWarning ? <p role="alert" className="text-xs text-[var(--warn)]">{integrationHubURLWarning}</p> : null}
        </div>
        {integrationHubURLError ? null : <CopyRow label="LabTether Hub URL" value={integrationHubURL.trim()} />}
        <CopyRow label="Token variable" value="LABTETHER_OWNER_TOKEN" />
        <p className="text-xs text-[var(--muted)]">
          Use an address that the Home Assistant host can reach; do not use localhost unless both products run on the same host.
        </p>
      </div>

      <div>
        <label htmlFor="homeassistant-base-url" className="mb-1 block text-xs font-medium text-[var(--muted)]">Home Assistant Base URL</label>
        <Input id="homeassistant-base-url" value={baseURL} onChange={(event) => setBaseURL(event.target.value)} placeholder="http://homeassistant.local:8123" />
      </div>

      <div>
        <label htmlFor="homeassistant-access-token" className="mb-1 block text-xs font-medium text-[var(--muted)]">Long-Lived Access Token</label>
        <Input
          id="homeassistant-access-token"
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
        <label htmlFor="homeassistant-display-name" className="mb-1 block text-xs font-medium text-[var(--muted)]">Display Name</label>
        <Input id="homeassistant-display-name" value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder="Home Assistant" />
      </div>

      {setupMode === "advanced" ? (
        <>
          <div>
            <label htmlFor="homeassistant-poll-interval" className="mb-1 block text-xs font-medium text-[var(--muted)]">Poll Interval (s)</label>
            <Input
              id="homeassistant-poll-interval"
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

import { useCallback, useEffect, useRef, useState } from "react";
import { Loader2 } from "lucide-react";
import { Button } from "../ui/Button";
import { Input } from "../ui/Input";
import { useToast } from "../../contexts/ToastContext";
import { useStatusControls } from "../../contexts/StatusContext";
import { sanitizeErrorMessage } from "../../lib/sanitizeErrorMessage";
import { baseURLHostLabel, validateHTTPSOrHTTPURL, validatePollIntervalSeconds, validatePortainerTokenID } from "./validation";
import { monitorCollectorRunWithRetry } from "./collectorSync";
import type { AddDeviceAddedEvent, AddDeviceCompatPrefill, SetupMode } from "./types";

type PortainerSettingsPayload = {
  configured?: boolean;
  collector_id?: string;
  credential_id?: string;
  credential_name?: string;
  settings?: {
    base_url?: string;
    auth_method?: "api_key" | "password";
    token_id?: string;
    cluster_name?: string;
    skip_verify?: boolean;
    interval_seconds?: number;
  };
  message?: string;
  warning?: string;
  result?: unknown;
  error?: string;
};

type PortainerSetupStepProps = {
  onBack: () => void;
  onClose: () => void;
  onAdded?: (event: AddDeviceAddedEvent) => void;
  compatPrefills?: AddDeviceCompatPrefill[];
  setupMode: SetupMode;
};

export function PortainerSetupStep({ onBack, onClose, onAdded, compatPrefills = [], setupMode }: PortainerSetupStepProps) {
  const { addToast } = useToast();
  const { fetchStatus } = useStatusControls();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [configured, setConfigured] = useState(false);
  const [credentialID, setCredentialID] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [tokenID, setTokenID] = useState("");
  const [tokenSecret, setTokenSecret] = useState("");
  const [clusterName, setClusterName] = useState("");
  const [skipVerify, setSkipVerify] = useState(false);
  const [intervalSeconds, setIntervalSeconds] = useState(60);
  const savingRef = useRef(false);
  const [formError, setFormError] = useState("");
  const prefillAppliedRef = useRef(false);
  const [selectedCompatBaseURL, setSelectedCompatBaseURL] = useState("");
  const selectedCompat = compatPrefills.find((item) => item.baseURL === selectedCompatBaseURL) ?? compatPrefills[0];

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    setMessage("");
    try {
      const response = await fetch("/api/settings/portainer", { cache: "no-store" });
      const payload = (await response.json()) as PortainerSettingsPayload;
      if (!response.ok) {
        throw new Error(payload.error || `failed to load portainer settings (${response.status})`);
      }
      const collectorID = payload.collector_id?.trim() ?? "";
      setConfigured(Boolean(payload.configured));
      setCredentialID(payload.credential_id ?? "");
      setBaseURL(payload.settings?.base_url ?? "");
      setTokenID(payload.settings?.token_id ?? "");
      setClusterName(payload.settings?.cluster_name ?? "");
      setSkipVerify(payload.settings?.skip_verify ?? false);
      setIntervalSeconds(payload.settings?.interval_seconds ?? 60);
      return collectorID;
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "failed to load portainer settings"));
      return "";
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (compatPrefills.length === 0) {
      setSelectedCompatBaseURL("");
      return;
    }
    if (!selectedCompatBaseURL) {
      setSelectedCompatBaseURL(compatPrefills[0].baseURL);
    }
  }, [compatPrefills, selectedCompatBaseURL]);

  const applyCompatPrefill = useCallback((prefill: AddDeviceCompatPrefill, forceName: boolean) => {
    setBaseURL(prefill.baseURL);
    if (forceName || !clusterName.trim()) {
      setClusterName(prefill.serviceName || "Homelab Portainer");
    }
  }, [clusterName, setBaseURL, setClusterName]);

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

  const handleTest = async () => {
    const testValidationError = validateTest();
    if (testValidationError) {
      setFormError(testValidationError);
      addToast("error", testValidationError);
      return;
    }

    setTesting(true);
    setError("");
    setMessage("");
    setFormError("");
    try {
      const body: Record<string, unknown> = {
        base_url: baseURL,
        auth_method: "api_key",
        token_id: tokenID,
        credential_id: credentialID,
        skip_verify: skipVerify
      };
      if (tokenSecret) {
        body.token_secret = tokenSecret;
      }
      const response = await fetch("/api/settings/portainer/test", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body)
      });
      const payload = (await response.json()) as PortainerSettingsPayload;
      if (!response.ok) {
        throw new Error(payload.error || `connection test failed (${response.status})`);
      }
      setMessage(payload.message || "Portainer connection succeeded.");
    } catch (err) {
      setError(
        sanitizeErrorMessage(
          err instanceof Error ? err.message : "",
          "failed to test portainer connection",
          [tokenSecret],
        ),
      );
    } finally {
      setTesting(false);
    }
  };

  const handleSave = async () => {
    if (savingRef.current) return;
    const saveValidationError = validateSave();
    if (saveValidationError) {
      setFormError(saveValidationError);
      addToast("error", saveValidationError);
      return;
    }

    savingRef.current = true;
    setSaving(true);
    setError("");
    setMessage("");
    setFormError("");
    try {
      const response = await fetch("/api/settings/portainer", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          base_url: baseURL,
          auth_method: "api_key",
          token_id: tokenID,
          token_secret: tokenSecret,
          cluster_name: clusterName,
          skip_verify: skipVerify,
          interval_seconds: intervalSeconds
        })
      });
      const payload = (await response.json()) as PortainerSettingsPayload;
      if (!response.ok) {
        throw new Error(payload.error || `failed to save portainer settings (${response.status})`);
      }

      const savedCollectorID = extractCollectorID(payload);
      setTokenSecret("");
      const loadedCollectorID = await load();
      const collectorID = savedCollectorID || loadedCollectorID;

      setMessage(payload.warning ? `Portainer connection settings saved. ${payload.warning}` : "Portainer connection settings saved.");
      if (payload.warning) {
        addToast("warning", payload.warning);
      }

      if (collectorID) {
        monitorCollectorRunWithRetry(collectorID, "Portainer", addToast);
      }

      const focusQuery = clusterName.trim() || baseURLHostLabel(baseURL);
      onAdded?.({ source: "portainer", focusQuery });
      addToast("success", "Portainer connector saved.");
      void fetchStatus();
      onClose();
    } catch (err) {
      const errorMessage = sanitizeErrorMessage(
        err instanceof Error ? err.message : "",
        "failed to save portainer settings",
        [tokenSecret],
      );
      setError(errorMessage);
      addToast("error", errorMessage);
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  };

  const validateTest = (): string => {
    const baseURLError = validateHTTPSOrHTTPURL(baseURL);
    const tokenIDError = validatePortainerTokenID(tokenID);
    return [baseURLError, tokenIDError].find((entry) => entry) ?? "";
  };

  const validateSave = (): string => {
    const baseURLError = validateHTTPSOrHTTPURL(baseURL);
    const tokenIDError = validatePortainerTokenID(tokenID);
    const intervalError = validatePollIntervalSeconds(intervalSeconds);
    const secretError = (configured || Boolean(credentialID) ? "" : !tokenSecret.trim() ? "API Key is required for initial setup." : "");
    return [baseURLError, tokenIDError, intervalError, secretError].find((entry) => entry) ?? "";
  };

  const canTest = !testing && !saving && !validateTest();
  const canSave = !saving && !testing && !validateSave();

  if (loading) {
    return <p className="text-xs text-[var(--muted)]">Loading Portainer settings...</p>;
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
          <p className="mt-1 text-xs text-[var(--muted)]">Selecting a detected endpoint updates base URL and cluster name.</p>
        </div>
      )}

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Base URL</label>
        <Input
          value={baseURL}
          onChange={(e) => setBaseURL(e.target.value)}
          placeholder="https://portainer.local:9443"
        />
      </div>

      <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2">
        <p className="text-xs font-medium text-[var(--text)]">Credential Template</p>
        <p className="text-xs text-[var(--muted)]">
          Use a Portainer API key with permission to list environments/endpoints. Token ID is optional and stored only as a label in LabTether.
        </p>
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Token ID (optional)</label>
        <Input
          value={tokenID}
          onChange={(e) => setTokenID(e.target.value)}
          placeholder="Friendly label for this key"
        />
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">API Key</label>
        <Input
          type="password"
          value={tokenSecret}
          onChange={(e) => setTokenSecret(e.target.value)}
          placeholder={configured ? "•••••••• (unchanged)" : "Required for initial setup"}
        />
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Cluster Name</label>
        <Input
          value={clusterName}
          onChange={(e) => setClusterName(e.target.value)}
          placeholder="Homelab Portainer"
        />
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
            <input
              type="checkbox"
              checked={skipVerify}
              onChange={(e) => setSkipVerify(e.target.checked)}
            />
            Skip TLS certificate verification
          </label>
        </>
      ) : (
        <p className="text-xs text-[var(--muted)]">Default setup uses 60s polling with TLS verification enabled.</p>
      )}

      <div className="flex items-center gap-3 pt-2">
        <Button onClick={onBack}>Back</Button>
        <Button disabled={!canTest} onClick={() => void handleTest()}>
          {testing ? <Loader2 size={14} className="animate-spin mr-1 inline" /> : null}
          {testing ? "Testing..." : "Test Connection"}
        </Button>
        <Button variant="primary" disabled={!canSave} onClick={() => void handleSave()}>
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

function extractCollectorID(payload: PortainerSettingsPayload): string {
  const fromRoot = payload.collector_id?.trim();
  if (fromRoot) return fromRoot;

  const result = payload.result;
  if (!result || typeof result !== "object") {
    return "";
  }

  const resultRecord = result as Record<string, unknown>;
  const collector = resultRecord.collector;
  if (collector && typeof collector === "object") {
    const collectorID = (collector as Record<string, unknown>).id;
    if (typeof collectorID === "string" && collectorID.trim()) {
      return collectorID.trim();
    }
  }

  const resultCollectorID = resultRecord.collector_id;
  if (typeof resultCollectorID === "string" && resultCollectorID.trim()) {
    return resultCollectorID.trim();
  }

  return "";
}

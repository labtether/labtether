import { useCallback, useEffect, useRef, useState } from "react";
import { Loader2 } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "../ui/Button";
import { Input } from "../ui/Input";
import { useToast } from "../../contexts/ToastContext";
import { useStatusControls } from "../../contexts/StatusContext";
import { readHubReply, useConnectorWords } from "../../hooks/connectorWords";
import { validatePollIntervalSeconds } from "./validation";
import { monitorCollectorRunWithRetry } from "./collectorSync";
import { compatPrefillOption, validationText } from "./addDeviceText";
import type { AddDeviceAddedEvent, AddDeviceCompatPrefill, SetupMode } from "./types";

type DockerSettingsPayload = {
  configured?: boolean;
  collector_id?: string;
  settings?: {
    cluster_name?: string;
    interval_seconds?: number;
  };
  error?: string;
  message?: string;
  warning?: string;
  result?: unknown;
};

type DockerSetupStepProps = {
  onBack: () => void;
  onClose: () => void;
  onAdded?: (event: AddDeviceAddedEvent) => void;
  compatPrefills?: AddDeviceCompatPrefill[];
  setupMode: SetupMode;
};

export function DockerSetupStep({ onBack, onClose, onAdded, compatPrefills = [], setupMode }: DockerSetupStepProps) {
  const t = useTranslations("auth.addDevice");
  const tSync = useTranslations("auth.addDevice.sync");
  const words = useConnectorWords("Docker");
  const { addToast } = useToast();
  const { fetchStatus } = useStatusControls();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [configured, setConfigured] = useState(false);
  const [clusterName, setClusterName] = useState("Docker Cluster");
  const [intervalSeconds, setIntervalSeconds] = useState(60);
  const savingRef = useRef(false);
  const prefillAppliedRef = useRef(false);
  const [selectedCompatBaseURL, setSelectedCompatBaseURL] = useState("");
  const [formError, setFormError] = useState("");
  const selectedCompat = compatPrefills.find((item) => item.baseURL === selectedCompatBaseURL) ?? compatPrefills[0];

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    setMessage("");
    try {
      const response = await fetch("/api/settings/docker", { cache: "no-store" });
      const payload = (await response.json()) as DockerSettingsPayload;
      if (!response.ok) {
        throw new Error(payload.error || t("form.loadFailed", { name: "Docker" }));
      }
      const collectorID = payload.collector_id?.trim() ?? "";
      setConfigured(Boolean(payload.configured));
      setClusterName(payload.settings?.cluster_name || "Docker Cluster");
      setIntervalSeconds(payload.settings?.interval_seconds ?? 60);
      return collectorID;
    } catch (err) {
      setError(err instanceof Error && err.message ? err.message : t("form.loadFailed", { name: "Docker" }));
      return "";
    } finally {
      setLoading(false);
    }
  }, [t]);

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

  useEffect(() => {
    if (prefillAppliedRef.current) return;
    if (loading) return;
    if (!selectedCompat) return;
    if (configured) {
      prefillAppliedRef.current = true;
      return;
    }

    if (!clusterName.trim() || clusterName.trim() === "Docker Cluster") {
      setClusterName(selectedCompat.serviceName || "Docker Cluster");
    }
    prefillAppliedRef.current = true;
  }, [loading, configured, selectedCompat, clusterName]);

  const handleTest = async () => {
    setTesting(true);
    setError("");
    setMessage("");
    try {
      const response = await fetch("/api/settings/docker/test", {
        method: "POST",
        headers: { "Content-Type": "application/json" }
      });
      await readHubReply<DockerSettingsPayload>(response);
      setMessage(words.testOk());
    } catch (err) {
      setError(words.testFailed(err));
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
      const response = await fetch("/api/settings/docker", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          cluster_name: clusterName,
          interval_seconds: intervalSeconds
        })
      });
      const payload = (await response.json()) as DockerSettingsPayload;
      if (!response.ok) {
        throw new Error(payload.error || t("form.saveFailed", { name: "Docker" }));
      }

      const savedCollectorID = extractCollectorID(payload);
      const loadedCollectorID = await load();
      const collectorID = savedCollectorID || loadedCollectorID;

      const saved = t("form.saved", { name: "Docker" });
      setMessage(payload.warning ? `${saved} ${payload.warning}` : saved);
      if (payload.warning) {
        addToast("warning", payload.warning);
      }

      if (collectorID) {
        monitorCollectorRunWithRetry(collectorID, "Docker", addToast, tSync);
      }

      onAdded?.({ source: "docker", focusQuery: clusterName.trim() });
      addToast("success", t("form.connectorSaved", { name: "Docker" }));
      void fetchStatus();
      onClose();
    } catch (err) {
      const errorMessage = err instanceof Error && err.message ? err.message : t("form.saveFailed", { name: "Docker" });
      setError(errorMessage);
      addToast("error", errorMessage);
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  };

  const validateSave = (): string => {
    if (!clusterName.trim()) {
      return t("validation.clusterNameRequired");
    }
    return validationText(t, validatePollIntervalSeconds(intervalSeconds));
  };

  if (loading) {
    return <p className="text-xs text-[var(--muted)]">{t("form.loading")}</p>;
  }

  return (
    <div className="space-y-3">
      {error && <p className="text-xs text-[var(--bad)]">{error}</p>}
      {message && <p className="text-xs text-[var(--ok)]">{message}</p>}
      {formError ? <p className="text-xs text-[var(--warn)]">{formError}</p> : null}

      {compatPrefills.length > 1 && (
        <div>
          <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.detected")}</label>
          <select
            value={selectedCompatBaseURL || compatPrefills[0].baseURL}
            onChange={(event) => {
              const nextBaseURL = event.target.value;
              setSelectedCompatBaseURL(nextBaseURL);
              const next = compatPrefills.find((item) => item.baseURL === nextBaseURL);
              if (next) {
                setClusterName(next.serviceName || "Docker Cluster");
              }
            }}
            className="w-full rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-2 text-xs text-[var(--text)]"
          >
            {compatPrefills.map((prefill) => (
              <option key={prefill.baseURL} value={prefill.baseURL}>
                {compatPrefillOption(t, prefill)}
              </option>
            ))}
          </select>
        </div>
      )}

      <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2">
        <p className="text-xs font-medium text-[var(--text)]">{t("form.credentials")}</p>
        <p className="text-xs text-[var(--muted)]">{t("docker.credentialsText")}</p>
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.clusterName")}</label>
        <Input
          value={clusterName}
          onChange={(e) => setClusterName(e.target.value)}
          placeholder="Homelab Docker"
        />
      </div>

      {setupMode === "advanced" ? (
        <div>
          <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.pollInterval")}</label>
          <Input
            type="number"
            min={15}
            max={3600}
            value={intervalSeconds}
            onChange={(e) => setIntervalSeconds(Number(e.target.value) || 60)}
          />
        </div>
      ) : (
        <p className="text-xs text-[var(--muted)]">{t("docker.defaultPolling")}</p>
      )}

      <div className="flex items-center gap-3 pt-2">
        <Button onClick={onBack}>{t("form.back")}</Button>
        <Button disabled={testing} onClick={() => void handleTest()}>
          {testing ? <Loader2 size={14} className="animate-spin mr-1 inline" /> : null}
          {testing ? t("form.testing") : t("form.testConnection")}
        </Button>
        <Button variant="primary" disabled={saving || Boolean(validateSave())} onClick={() => void handleSave()}>
          {saving ? <Loader2 size={14} className="animate-spin mr-1 inline" /> : null}
          {saving ? t("form.saving") : t("form.saveAndSync")}
        </Button>
      </div>
    </div>
  );
}

function extractCollectorID(payload: DockerSettingsPayload): string {
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

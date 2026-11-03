"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Loader2 } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "../ui/Button";
import { Input } from "../ui/Input";
import { useTrueNASSettings } from "../../hooks/useTrueNASSettings";
import { useToast } from "../../contexts/ToastContext";
import { useStatusControls } from "../../contexts/StatusContext";
import { baseURLHostLabel, validateHTTPSOrHTTPURL, validatePollIntervalSeconds } from "./validation";
import { monitorCollectorRunWithRetry } from "./collectorSync";
import { compatPrefillOption, firstError, validationText } from "./addDeviceText";
import type { AddDeviceAddedEvent, AddDeviceCompatPrefill, SetupMode } from "./types";

type TrueNASSetupStepProps = {
  onBack: () => void;
  onClose: () => void;
  onAdded?: (event: AddDeviceAddedEvent) => void;
  compatPrefills?: AddDeviceCompatPrefill[];
  setupMode: SetupMode;
};

export function TrueNASSetupStep({ onBack, onClose, onAdded, compatPrefills = [], setupMode }: TrueNASSetupStepProps) {
  const t = useTranslations("auth.addDevice");
  const tSync = useTranslations("auth.addDevice.sync");
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

  const baseURLError = validationText(t, validateHTTPSOrHTTPURL(baseURL));
  const intervalError = validationText(t, validatePollIntervalSeconds(intervalSeconds));
  const apiKeyError = !apiKey.trim() ? t("validation.apiKeyRequired") : "";
  const saveValidationError = firstError(baseURLError, intervalError, apiKeyError);
  const testValidationError = firstError(baseURLError, apiKeyError);

  const handleSave = async () => {
    if (saveValidationError) {
      setFormError(saveValidationError);
      addToast("error", saveValidationError);
      return;
    }
    setFormError("");

    const result = await save();
    if (!result.ok) {
      addToast("error", result.error || t("form.saveFailed", { name: "TrueNAS" }));
      return;
    }

    if (result.warning) {
      addToast("warning", result.warning);
    }

    if (result.collectorID) {
      monitorCollectorRunWithRetry(result.collectorID, "TrueNAS", addToast, tSync);
    }

    const focusQuery = displayName.trim() || baseURLHostLabel(baseURL);
    onAdded?.({ source: "truenas", focusQuery });
    addToast("success", t("form.connectorSaved", { name: "TrueNAS" }));
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
                applyCompatPrefill(next, true);
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

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.baseUrl")}</label>
        <Input value={baseURL} onChange={(e) => setBaseURL(e.target.value)} placeholder="https://truenas.local" />
      </div>

      <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2">
        <p className="text-xs font-medium text-[var(--text)]">{t("form.credentials")}</p>
        <p className="text-xs text-[var(--muted)]">{t("truenas.credentialsText")}</p>
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.apiKey")}</label>
        <Input
          type="password"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          placeholder={t("form.required")}
        />
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.displayName")}</label>
        <Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder="Homelab TrueNAS" />
      </div>

      {setupMode === "advanced" ? (
        <>
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

          <label className="flex items-center gap-2 text-xs text-[var(--muted)]">
            <input type="checkbox" checked={skipVerify} onChange={(e) => setSkipVerify(e.target.checked)} />
            {t("form.skipTls")}
          </label>
        </>
      ) : (
        <p className="text-xs text-[var(--muted)]">{t("form.defaultPolling")}</p>
      )}

      <div className="flex items-center gap-3 pt-2">
        <Button onClick={onBack}>{t("form.back")}</Button>
        <Button disabled={testing || saving || running} onClick={() => void handleTestConnection()}>
          {testing ? <Loader2 size={14} className="animate-spin mr-1 inline" /> : null}
          {testing ? t("form.testing") : t("form.testConnection")}
        </Button>
        <Button disabled={!collectorID || saving || testing || running} onClick={() => void runNow()}>
          {running ? <Loader2 size={14} className="animate-spin mr-1 inline" /> : null}
          {running ? t("form.starting") : t("form.syncNow")}
        </Button>
        <Button variant="primary" disabled={saving || testing || running || Boolean(saveValidationError)} onClick={() => void handleSave()}>
          {saving ? <Loader2 size={14} className="animate-spin mr-1 inline" /> : null}
          {saving ? t("form.saving") : t("form.saveAndSync")}
        </Button>
      </div>
    </div>
  );
}

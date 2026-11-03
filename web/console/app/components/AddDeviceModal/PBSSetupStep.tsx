"use client";

import { useCallback, useEffect, useId, useRef, useState } from "react";
import { Loader2 } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "../ui/Button";
import { Input } from "../ui/Input";
import { usePBSSettings } from "../../hooks/usePBSSettings";
import { useToast } from "../../contexts/ToastContext";
import { useStatusControls } from "../../contexts/StatusContext";
import { baseURLHostLabel, validateHTTPSOrHTTPURL, validatePollIntervalSeconds, validateProxmoxTokenID } from "./validation";
import { monitorCollectorRunWithRetry } from "./collectorSync";
import { compatPrefillOption, firstError, validationText } from "./addDeviceText";
import type { AddDeviceAddedEvent, AddDeviceCompatPrefill, SetupMode } from "./types";

type PBSSetupStepProps = {
  onBack: () => void;
  onClose: () => void;
  onAdded?: (event: AddDeviceAddedEvent) => void;
  compatPrefills?: AddDeviceCompatPrefill[];
  setupMode: SetupMode;
};

export function PBSSetupStep({ onBack, onClose, onAdded, compatPrefills = [], setupMode }: PBSSetupStepProps) {
  const t = useTranslations("auth.addDevice");
  const tSync = useTranslations("auth.addDevice.sync");
  const { addToast } = useToast();
  const { fetchStatus } = useStatusControls();
  const {
    baseURL,
    setBaseURL,
    tokenID,
    setTokenID,
    tokenSecret,
    setTokenSecret,
    displayName,
    setDisplayName,
    intervalSeconds,
    setIntervalSeconds,
    skipVerify,
    setSkipVerify,
    caPEM,
    setCAPEM,
    collectorID,
    credentialID,
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
  } = usePBSSettings();

  const [formError, setFormError] = useState("");
  const fieldID = useId();
  const prefillAppliedRef = useRef(false);
  const [selectedCompatBaseURL, setSelectedCompatBaseURL] = useState("");
  const selectedCompat = compatPrefills.find((item) => item.baseURL === selectedCompatBaseURL) ?? compatPrefills[0];

  const applyCompatPrefill = useCallback((prefill: AddDeviceCompatPrefill, forceName: boolean) => {
    setBaseURL(prefill.baseURL);
    if (forceName || !displayName.trim()) {
      setDisplayName(prefill.serviceName || "Homelab PBS");
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
  const tokenIDError = validationText(t, validateProxmoxTokenID(tokenID));
  const intervalError = validationText(t, validatePollIntervalSeconds(intervalSeconds));
  const secretRequiredError = (configured || Boolean(credentialID) || tokenSecret.trim())
    ? ""
    : t("validation.tokenSecretRequired");
  const saveValidationError = firstError(baseURLError, tokenIDError, intervalError, secretRequiredError);
  const testValidationError = firstError(baseURLError, tokenIDError);

  const handleSave = async () => {
    if (saveValidationError) {
      setFormError(saveValidationError);
      addToast("error", saveValidationError);
      return;
    }
    setFormError("");

    const result = await save();
    if (!result.ok) {
      addToast("error", result.error || t("form.saveFailed", { name: "PBS" }));
      return;
    }

    if (result.warning) {
      addToast("warning", result.warning);
    }

    if (result.collectorID) {
      monitorCollectorRunWithRetry(result.collectorID, "PBS", addToast, tSync);
    }

    const focusQuery = displayName.trim() || baseURLHostLabel(baseURL);
    onAdded?.({ source: "pbs", focusQuery });
    addToast("success", t("form.connectorSaved", { name: "PBS" }));
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
          <label htmlFor={`${fieldID}-detected-endpoint`} className="text-xs font-medium text-[var(--muted)] mb-1 block">
            {t("form.detected")}
          </label>
          <select
            id={`${fieldID}-detected-endpoint`}
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
        <label htmlFor={`${fieldID}-base-url`} className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.baseUrl")}</label>
        <Input
          id={`${fieldID}-base-url`}
          value={baseURL}
          onChange={(e) => setBaseURL(e.target.value)}
          placeholder="https://pbs.local:8007"
          aria-invalid={Boolean(baseURLError)}
        />
      </div>

      <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2">
        <p className="text-xs font-medium text-[var(--text)]">{t("form.credentials")}</p>
        <p className="text-xs text-[var(--muted)]">
          {t.rich("pbs.credentialsText", { code: (chunks) => <code className="text-[var(--text)]">{chunks}</code> })}
        </p>
      </div>

      <div>
        <label htmlFor={`${fieldID}-token-id`} className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.tokenId")}</label>
        <Input
          id={`${fieldID}-token-id`}
          value={tokenID}
          onChange={(e) => setTokenID(e.target.value)}
          placeholder="root@pam!labtether"
          autoComplete="off"
          aria-invalid={Boolean(tokenIDError)}
        />
      </div>

      <div>
        <label htmlFor={`${fieldID}-token-secret`} className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.tokenSecret")}</label>
        <Input
          id={`${fieldID}-token-secret`}
          type="password"
          value={tokenSecret}
          onChange={(e) => setTokenSecret(e.target.value)}
          placeholder={configured ? t("form.savedSecret") : t("form.requiredForSetup")}
          autoComplete="off"
          aria-invalid={Boolean(secretRequiredError)}
        />
      </div>

      <div>
        <label htmlFor={`${fieldID}-display-name`} className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.displayName")}</label>
        <Input
          id={`${fieldID}-display-name`}
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
          placeholder="Homelab PBS"
          autoComplete="off"
        />
      </div>

      {setupMode === "advanced" ? (
        <>
          <div>
            <label htmlFor={`${fieldID}-poll-interval`} className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.pollInterval")}</label>
            <Input
              id={`${fieldID}-poll-interval`}
              type="number"
              min={15}
              max={3600}
              value={intervalSeconds}
              onChange={(e) => setIntervalSeconds(Number(e.target.value) || 60)}
              aria-invalid={Boolean(intervalError)}
            />
          </div>

          <label htmlFor={`${fieldID}-skip-verify`} className="flex items-center gap-2 text-xs text-[var(--muted)]">
            <input
              id={`${fieldID}-skip-verify`}
              type="checkbox"
              checked={skipVerify}
              onChange={(e) => setSkipVerify(e.target.checked)}
            />
            {t("form.skipTls")}
          </label>

          <div>
            <label htmlFor={`${fieldID}-ca-pem`} className="text-xs font-medium text-[var(--muted)] mb-1 block">
              {t("pbs.caPem")}
            </label>
            <textarea
              id={`${fieldID}-ca-pem`}
              className="w-full bg-transparent border border-[var(--line)] rounded-lg px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:outline-none focus:border-[var(--text-3)] focus:shadow-[0_0_0_3px_var(--ring)] transition-[border-color,box-shadow] duration-[var(--dur-fast)] resize-y"
              rows={4}
              value={caPEM}
              onChange={(e) => setCAPEM(e.target.value)}
              placeholder="-----BEGIN CERTIFICATE-----"
              autoComplete="off"
              spellCheck={false}
            />
            <p className="mt-1 text-xs text-[var(--muted)]">{t("pbs.caPemHint")}</p>
          </div>
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

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../ui/Button";
import { Input } from "../ui/Input";
import { useProxmoxSettings } from "../../hooks/useProxmoxSettings";
import { useToast } from "../../contexts/ToastContext";
import { useStatusControls } from "../../contexts/StatusContext";
import { baseURLHostLabel, validateHTTPSOrHTTPURL, validatePollIntervalSeconds, validateProxmoxTokenID } from "./validation";
import { monitorCollectorRunWithRetry } from "./collectorSync";
import { compatPrefillOption, firstError, validationText } from "./addDeviceText";
import type { AddDeviceAddedEvent, AddDeviceCompatPrefill, SetupMode } from "./types";

type ProxmoxSetupStepProps = {
  onBack: () => void;
  onClose: () => void;
  onAdded?: (event: AddDeviceAddedEvent) => void;
  compatPrefills?: AddDeviceCompatPrefill[];
  setupMode: SetupMode;
};

export function ProxmoxSetupStep({ onBack, onClose, onAdded, compatPrefills = [], setupMode }: ProxmoxSetupStepProps) {
  const t = useTranslations("auth.addDevice");
  const tSync = useTranslations("auth.addDevice.sync");
  const { addToast } = useToast();
  const { fetchStatus } = useStatusControls();
  const {
    baseURL, setBaseURL,
    authMethod, setAuthMethod,
    tokenID, setTokenID,
    tokenSecret, setTokenSecret,
    username, setUsername,
    skipVerify, setSkipVerify,
    spiceSkipVerify, setSPICESkipVerify,
    clusterName, setClusterName,
    intervalSeconds, setIntervalSeconds,
    collectorID,
    configured,
    loading,
    saving, testing, running,
    error, message,
    save, testConnection, runNow,
  } = useProxmoxSettings();

  const baseURLError = validationText(t, validateHTTPSOrHTTPURL(baseURL));
  const intervalError = validationText(t, validatePollIntervalSeconds(intervalSeconds));
  const tokenIDError = authMethod === "api_token" ? validationText(t, validateProxmoxTokenID(tokenID)) : "";
  const usernameError = authMethod === "password" && !username.trim() ? t("validation.usernameRequired") : "";

  const saveValidationError = firstError(baseURLError, intervalError, tokenIDError, usernameError);
  const testValidationError = firstError(baseURLError, tokenIDError, usernameError);

  const handleSave = async () => {
    if (saveValidationError) {
      setFormError(saveValidationError);
      addToast("error", saveValidationError);
      return;
    }
    setFormError("");

    const result = await save();
    if (!result.ok) {
      addToast("error", result.error || t("form.saveFailed", { name: "Proxmox" }));
      return;
    }

    if (result.warning) {
      addToast("warning", result.warning);
    }

    if (result.collectorID) {
      monitorCollectorRunWithRetry(result.collectorID, "Proxmox", addToast, tSync);
    }

    const focusQuery = clusterName.trim() || baseURLHostLabel(baseURL);
    onAdded?.({ source: "proxmox", focusQuery });
    addToast("success", t("form.connectorSaved", { name: "Proxmox" }));
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
        <Input value={baseURL} onChange={(e) => setBaseURL(e.target.value)} placeholder="https://pve.local:8006" />
      </div>

      <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2">
        <p className="text-xs font-medium text-[var(--text)]">{t("form.credentials")}</p>
        <p className="text-xs text-[var(--muted)]">
          {t.rich("proxmox.credentialsText", { code: (chunks) => <code className="text-[var(--text)]">{chunks}</code> })}
        </p>
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1.5 block">{t("proxmox.auth")}</label>
        <div className="flex gap-2">
          <AuthMethodButton
            active={authMethod === "api_token"}
            onClick={() => setAuthMethod("api_token")}
            label={t("proxmox.apiToken")}
          />
          <AuthMethodButton
            active={authMethod === "password"}
            onClick={() => setAuthMethod("password")}
            label={t("proxmox.userPassword")}
          />
        </div>
      </div>

      {authMethod === "api_token" ? (
        <>
          <div>
            <p className="text-xs text-[var(--warn-text)]">
              {t("proxmox.consoleLimit")}
            </p>
          </div>
          <div>
            <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.tokenId")}</label>
            <Input value={tokenID} onChange={(e) => setTokenID(e.target.value)} placeholder="labtether@pve!monitoring" />
          </div>
          <div>
            <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.tokenSecret")}</label>
            <Input
              type="password"
              value={tokenSecret}
              onChange={(e) => setTokenSecret(e.target.value)}
              placeholder={configured ? t("form.savedSecret") : t("form.requiredForSetup")}
            />
          </div>
        </>
      ) : (
        <>
          <div>
            <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.username")}</label>
            <Input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="root@pam" />
          </div>
          <div>
            <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.password")}</label>
            <Input
              type="password"
              value={tokenSecret}
              onChange={(e) => setTokenSecret(e.target.value)}
              placeholder={configured ? t("form.savedSecret") : t("form.requiredForSetup")}
            />
          </div>
        </>
      )}

      {setupMode === "advanced" ? (
        <>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.clusterName")}</label>
              <Input value={clusterName} onChange={(e) => setClusterName(e.target.value)} placeholder="Homelab Proxmox" />
            </div>
            <div>
              <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.pollInterval")}</label>
              <Input type="number" min={15} max={3600} value={intervalSeconds} onChange={(e) => setIntervalSeconds(Number(e.target.value) || 60)} />
            </div>
          </div>

          <label className="flex items-center gap-2 text-xs text-[var(--muted)]">
            <input type="checkbox" checked={skipVerify} onChange={(e) => setSkipVerify(e.target.checked)} />
            {t("proxmox.skipApiTls")}
          </label>
          {skipVerify && !spiceSkipVerify && (
            <p className="text-xs text-[var(--warn)]">{t("proxmox.spiceBlocked")}</p>
          )}
          <label className="flex items-start gap-2 text-xs text-[var(--warn)]">
            <input
              type="checkbox"
              checked={spiceSkipVerify}
              onChange={(e) => setSPICESkipVerify(e.target.checked)}
              className="mt-0.5"
            />
            <span>{t("proxmox.skipSpiceTls")}</span>
          </label>
        </>
      ) : (
        <div>
          <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("form.clusterName")}</label>
          <Input value={clusterName} onChange={(e) => setClusterName(e.target.value)} placeholder="Homelab Proxmox" />
          <p className="mt-1 text-xs text-[var(--muted)]">{t("form.defaultPolling")}</p>
        </div>
      )}

      <div className="flex items-center gap-3 pt-2">
        <Button onClick={onBack}>{t("form.back")}</Button>
        <Button disabled={testing || saving || running} onClick={() => void handleTestConnection()}>
          {testing ? t("form.testing") : t("form.testConnection")}
        </Button>
        <Button disabled={!collectorID || saving || testing || running} onClick={() => void runNow()}>
          {running ? t("form.starting") : t("form.syncNow")}
        </Button>
        <Button variant="primary" disabled={saving || testing || running || Boolean(saveValidationError)} onClick={() => void handleSave()}>
          {saving ? t("form.saving") : t("form.saveAndSync")}
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
          ? "border-transparent bg-[var(--sel)] text-[var(--text)]"
          : "border-[var(--line)] bg-transparent text-[var(--text-2)] hover:text-[var(--text)]"
      }`}
    >
      {label}
    </button>
  );
}

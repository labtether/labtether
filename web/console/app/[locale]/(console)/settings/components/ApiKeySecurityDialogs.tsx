"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import type { ApiKeyInfo, CreatedKeyResponse } from "../../../../hooks/useApiKeys";
import { sanitizeErrorMessage } from "../../../../lib/sanitizeErrorMessage";
import { SettingsDialog } from "./SettingsKit";
import { copyText } from "../../../../lib/clipboard";

/* ── a new key's value, shown once ── */

export type KeyRevealModalProps = {
  created: CreatedKeyResponse;
  onDismiss: () => void;
};

export function KeyRevealModal({ created, onDismiss }: KeyRevealModalProps) {
  const t = useTranslations("settings");
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await copyText(created.raw_key);
      setCopied(true);
      setTimeout(() => setCopied(false), 3000);
    } catch {
      // Copy refused: the value stays selectable.
    }
  };

  // The value is shown once, so a stray click outside does not close it; the button does.
  return (
    <SettingsDialog labelledBy="api-key-reveal-title" onClose={() => undefined}>
      <h3 id="api-key-reveal-title">{t("apiKeys.revealTitle", { name: created.name })}</h3>
      <code className="st-secret">{created.raw_key}</code>
      <p className="st-msg warn">{t("apiKeys.revealWarning")}</p>
      <div className="st-dlg-acts">
        <Button onClick={() => { void handleCopy(); }}>
          {copied ? t("apiKeys.revealCopied") : t("apiKeys.copy")}
        </Button>
        <Button variant="primary" onClick={onDismiss}>
          {t("apiKeys.revealDismiss")}
        </Button>
      </div>
    </SettingsDialog>
  );
}

/* ── revoke confirm ── */

export type RevokeConfirmModalProps = {
  keyInfo: ApiKeyInfo;
  onClose: () => void;
  onConfirm: () => Promise<void>;
};

export function RevokeConfirmModal({ keyInfo, onClose, onConfirm }: RevokeConfirmModalProps) {
  const t = useTranslations("settings");
  const [revoking, setRevoking] = useState(false);
  const [error, setError] = useState("");

  const handleConfirm = async () => {
    setRevoking(true);
    setError("");
    try {
      await onConfirm();
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", t("apiKeys.revokeFailed")));
      setRevoking(false);
    }
  };

  return (
    <SettingsDialog labelledBy="api-key-revoke-title" onClose={() => { if (!revoking) onClose(); }}>
      <h3 id="api-key-revoke-title">{t("apiKeys.revokeTitle", { name: keyInfo.name })}</h3>
      <p>{t("apiKeys.revokeBody", { name: keyInfo.name })}</p>
      {error ? <p className="st-msg bad" role="alert">{error}</p> : null}
      <div className="st-dlg-acts">
        <Button onClick={onClose} disabled={revoking}>{t("apiKeys.cancel")}</Button>
        <Button variant="danger" className="st-danger-fill" loading={revoking} onClick={() => { void handleConfirm(); }}>
          {t("apiKeys.revokeConfirm")}
        </Button>
      </div>
    </SettingsDialog>
  );
}

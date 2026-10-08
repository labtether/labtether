"use client";

import { useState } from "react";
import { Check, Copy, Key, ShieldAlert } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import type { ApiKeyInfo, CreatedKeyResponse } from "../../../../hooks/useApiKeys";
import { sanitizeErrorMessage } from "../../../../lib/sanitizeErrorMessage";

/* ── key reveal modal ── */

export type KeyRevealModalProps = {
  created: CreatedKeyResponse;
  onDismiss: () => void;
};

export function KeyRevealModal({ created, onDismiss }: KeyRevealModalProps) {
  const t = useTranslations("settings");
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(created.raw_key);
      setCopied(true);
      setTimeout(() => setCopied(false), 3000);
    } catch {
      // fallback: do nothing
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="api-key-reveal-title"
        onClick={(e) => e.stopPropagation()}
      >
        <Card className="w-[32rem] max-w-[92vw] space-y-4">
          <div className="flex items-center gap-2">
            <Key size={16} className="text-[var(--accent)]" />
            <h3 id="api-key-reveal-title" className="text-sm font-medium text-[var(--text)]">{t("apiKeys.revealTitle")}</h3>
          </div>
          <div className="bg-[var(--surface)] rounded-lg p-3 font-mono text-xs text-[var(--text)] break-all select-all">
            {created.raw_key}
          </div>
          <div className="flex items-center gap-2">
            <Button variant="secondary" size="sm" onClick={() => { void handleCopy(); }}>
              {copied ? <Check size={13} /> : <Copy size={13} />}
              {copied ? t("apiKeys.revealCopied") : "Copy"}
            </Button>
          </div>
          <p className="text-xs text-[var(--bad)] flex items-center gap-1.5">
            <ShieldAlert size={13} className="shrink-0" />
            {t("apiKeys.revealWarning")}
          </p>
          <div className="flex justify-end">
            <Button variant="primary" size="sm" onClick={onDismiss}>
              {t("apiKeys.revealDismiss")}
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

/* ── revoke confirm modal ── */

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
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "Failed to revoke key."));
      setRevoking(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={() => { if (!revoking) onClose(); }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="api-key-revoke-title"
        onClick={(e) => e.stopPropagation()}
      >
        <Card className="w-[28rem] max-w-[92vw] space-y-4">
          <h3 id="api-key-revoke-title" className="text-sm font-medium text-[var(--text)]">{t("apiKeys.revokeTitle")}</h3>
          <p className="text-xs text-[var(--muted)]">
            {t("apiKeys.revokeBody", { name: keyInfo.name })}
          </p>
          {error ? <p className="text-xs text-[var(--bad)]">{error}</p> : null}
          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onClick={onClose} disabled={revoking}>{t("apiKeys.cancel")}</Button>
            <Button variant="danger" loading={revoking} onClick={() => { void handleConfirm(); }}>{t("apiKeys.revokeConfirm")}</Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

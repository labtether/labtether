"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Facts, Section, Sheet } from "./SettingsKit";
import { SSHHubKeyRotateDialog } from "./SSHHubKeyRotateDialog";
import {
  parseSSHHubKeyInfo,
  sshHubKeyErrorMessage,
  type SSHHubKeyInfo,
  type SSHHubKeyRotationPayload,
} from "./sshHubKeyModel";
import { copyText } from "../../../../lib/clipboard";

async function responseJSON(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

export function SSHHubKeyCard() {
  const t = useTranslations("settings.sshKey");
  const [keyInfo, setKeyInfo] = useState<SSHHubKeyInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [copied, setCopied] = useState(false);
  const [rotateOpen, setRotateOpen] = useState(false);
  const [rotating, setRotating] = useState(false);
  const [rotationError, setRotationError] = useState("");
  const [message, setMessage] = useState("");

  const loadKeyInfo = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    setLoadError("");
    try {
      const response = await fetch("/api/settings/ssh-hub-key", {
        cache: "no-store",
        signal,
      });
      const payload = await responseJSON(response);
      if (!response.ok) {
        throw new Error(sshHubKeyErrorMessage(payload, t("loadFailed")));
      }
      const parsed = parseSSHHubKeyInfo(payload);
      if (!parsed) throw new Error(t("loadFailed"));
      setKeyInfo(parsed);
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      setLoadError(error instanceof Error ? error.message : t("loadFailed"));
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    const controller = new AbortController();
    void loadKeyInfo(controller.signal);
    return () => controller.abort();
  }, [loadKeyInfo]);

  async function copyPublicKey() {
    if (!keyInfo) return;
    setMessage("");
    setRotationError("");
    try {
      await copyText(keyInfo.public_key.trim());
      setCopied(true);
      setMessage(t("copiedMessage"));
    } catch {
      setCopied(false);
      setRotationError(t("copyFailed"));
    }
  }

  async function rotateKey(payload: SSHHubKeyRotationPayload): Promise<boolean> {
    setRotating(true);
    setRotationError("");
    setMessage("");
    try {
      const response = await fetch("/api/settings/ssh-hub-key", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      const responsePayload = await responseJSON(response);
      if (!response.ok) {
        setRotationError(sshHubKeyErrorMessage(responsePayload, t("rotateFailed")));
        return false;
      }
      const rotatedInfo = parseSSHHubKeyInfo(responsePayload);
      if (!rotatedInfo) {
        setRotationError(t("rotateUnclear"));
        return false;
      }
      setKeyInfo(rotatedInfo);
      setCopied(false);
      const agentSummary = typeof rotatedInfo.agents_updated === "number" && typeof rotatedInfo.agents_total === "number"
        ? ` ${t("agentsUpdated", { updated: rotatedInfo.agents_updated, total: rotatedInfo.agents_total })}`
        : "";
      setMessage(rotatedInfo.warning ? t("rotatedWarning", { warning: rotatedInfo.warning }) : `${t("rotated")}${agentSummary}`);
      return true;
    } catch {
      setRotationError(t("rotateFailed"));
      return false;
    } finally {
      setRotating(false);
    }
  }

  function closeRotateDialog() {
    if (rotating) return;
    setRotateOpen(false);
    setRotationError("");
  }

  return (
    <>
      <Section
        title={t("title")}
        id="hub-ssh-key"
        action={keyInfo ? (
          <Button
            variant="danger"
            size="sm"
            onClick={() => {
              setRotationError("");
              setMessage("");
              setRotateOpen(true);
            }}
          >
            {t("rotateButton")}
          </Button>
        ) : null}
      >
        <Sheet className={loading ? "st-loading" : ""}>
          {!loading && loadError ? (
            <div className="st-row">
              <p role="alert" className="st-msg bad">{loadError}</p>
              <div className="st-row-r">
                <Button size="sm" onClick={() => { void loadKeyInfo(); }}>{t("retry")}</Button>
              </div>
            </div>
          ) : null}

          {!loading && keyInfo ? (
            <>
              <Facts
                items={[
                  [t("type"), keyInfo.key_type === "rsa" ? "RSA 4096" : "Ed25519"],
                  [t("fingerprint"), <code key="fp" className="st-mono">{keyInfo.fingerprint_sha256}</code>],
                ]}
              />
              <div className="st-pad-row">
                <div className="st-code">
                  <pre aria-label={t("publicKey")}>{keyInfo.public_key.trim()}</pre>
                </div>
              </div>
            </>
          ) : null}

          {keyInfo || message || (!rotateOpen && rotationError) ? (
            <div className="st-foot" aria-live="polite">
              {message ? <p className="st-msg">{message}</p> : null}
              {!rotateOpen && rotationError ? <p role="alert" className="st-msg bad">{rotationError}</p> : null}
              {keyInfo ? (
                <Button size="sm" onClick={() => { void copyPublicKey(); }}>
                  {copied ? t("copied") : t("copy")}
                </Button>
              ) : null}
            </div>
          ) : null}
        </Sheet>
      </Section>

      {rotateOpen && keyInfo ? (
        <SSHHubKeyRotateDialog
          currentKeyType={keyInfo.key_type}
          loading={rotating}
          error={rotationError}
          onClose={closeRotateDialog}
          onConfirm={rotateKey}
        />
      ) : null}
    </>
  );
}

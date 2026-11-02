"use client";

import { useCallback, useEffect, useState } from "react";
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
        throw new Error(sshHubKeyErrorMessage(payload, `Failed to load SSH hub key (${response.status})`));
      }
      const parsed = parseSSHHubKeyInfo(payload);
      if (!parsed) throw new Error("SSH hub key endpoint returned an invalid response");
      setKeyInfo(parsed);
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      setLoadError(error instanceof Error ? error.message : "Failed to load SSH hub key");
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, []);

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
      setMessage("Public key copied to clipboard.");
    } catch {
      setCopied(false);
      setRotationError("The browser could not copy the public key. Select it manually instead.");
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
        setRotationError(sshHubKeyErrorMessage(
          responsePayload,
          `SSH hub key rotation failed (${response.status}); the displayed key remains active.`,
        ));
        return false;
      }
      const rotatedInfo = parseSSHHubKeyInfo(responsePayload);
      if (!rotatedInfo) {
        setRotationError("Rotation completed but the hub returned invalid key metadata. Reload before rotating again.");
        return false;
      }
      setKeyInfo(rotatedInfo);
      setCopied(false);
      const agentSummary = typeof rotatedInfo.agents_updated === "number" && typeof rotatedInfo.agents_total === "number"
        ? ` ${rotatedInfo.agents_updated} of ${rotatedInfo.agents_total} connected agents were updated.`
        : "";
      setMessage(rotatedInfo.warning ? `Key rotated with a warning: ${rotatedInfo.warning}` : `SSH hub key rotated.${agentSummary}`);
      return true;
    } catch {
      setRotationError("The rotation endpoint is unavailable; the displayed key remains active.");
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
        title="Hub SSH key"
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
            Rotate key…
          </Button>
        ) : null}
      >
        <Sheet className={loading ? "st-loading" : ""}>
          {!loading && loadError ? (
            <div className="st-row">
              <p role="alert" className="st-msg bad">{loadError}</p>
              <div className="st-row-r">
                <Button size="sm" onClick={() => { void loadKeyInfo(); }}>Retry</Button>
              </div>
            </div>
          ) : null}

          {!loading && keyInfo ? (
            <>
              <Facts
                items={[
                  ["Type", keyInfo.key_type === "rsa" ? "RSA 4096" : "Ed25519"],
                  ["Fingerprint", <code key="fp" className="st-mono">{keyInfo.fingerprint_sha256}</code>],
                ]}
              />
              <div className="st-pad-row">
                <div className="st-code">
                  <pre aria-label="OpenSSH public key">{keyInfo.public_key.trim()}</pre>
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
                  {copied ? "Copied" : "Copy public key"}
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

"use client";

import { useCallback, useEffect, useState } from "react";
import { Copy, KeyRound, RotateCw } from "lucide-react";
import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { SkeletonRow } from "../../../../components/ui/Skeleton";
import { SSHHubKeyRotateDialog } from "./SSHHubKeyRotateDialog";
import {
  parseSSHHubKeyInfo,
  sshHubKeyErrorMessage,
  type SSHHubKeyInfo,
  type SSHHubKeyRotationPayload,
} from "./sshHubKeyModel";

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
      await navigator.clipboard.writeText(keyInfo.public_key.trim());
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
      <Card className="mb-6">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex min-w-0 items-start gap-3">
            <KeyRound className="mt-0.5 shrink-0 text-[var(--accent)]" size={18} aria-hidden="true" />
            <div>
              <h2>SSH Hub Key</h2>
              <p className="mt-1 text-sm text-[var(--muted)]">
                Public identity used for passwordless hub-to-agent SSH access. Private key material is never shown here.
              </p>
            </div>
          </div>
          {keyInfo ? <Badge status={keyInfo.key_type === "rsa" ? "RSA 4096" : "Ed25519"} size="sm" /> : null}
        </div>

        {loading ? (
          <div className="mt-4 space-y-1" aria-label="Loading SSH hub key">
            <SkeletonRow />
            <SkeletonRow />
          </div>
        ) : null}

        {!loading && loadError ? (
          <div className="mt-4 flex flex-wrap items-center gap-3">
            <p role="alert" className="text-sm text-[var(--bad)]">{loadError}</p>
            <Button variant="secondary" size="sm" onClick={() => { void loadKeyInfo(); }}>
              Retry
            </Button>
          </div>
        ) : null}

        {!loading && keyInfo ? (
          <div className="mt-4 divide-y divide-[var(--line)]">
            <div className="py-3">
              <p className="text-xs font-medium text-[var(--muted)]">SHA-256 fingerprint</p>
              <code className="mt-1 block break-all text-sm text-[var(--text)]">
                {keyInfo.fingerprint_sha256}
              </code>
            </div>
            <div className="py-3">
              <p className="text-xs font-medium text-[var(--muted)]">OpenSSH public key</p>
              <code className="mt-1 block max-h-28 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-[var(--surface)] p-3 text-xs text-[var(--text)]">
                {keyInfo.public_key.trim()}
              </code>
              <div className="mt-3 flex flex-wrap gap-2">
                <Button variant="secondary" size="sm" onClick={() => { void copyPublicKey(); }}>
                  <Copy size={13} aria-hidden="true" />
                  {copied ? "Copied" : "Copy public key"}
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  onClick={() => {
                    setRotationError("");
                    setMessage("");
                    setRotateOpen(true);
                  }}
                >
                  <RotateCw size={13} aria-hidden="true" />
                  Rotate key
                </Button>
              </div>
            </div>
          </div>
        ) : null}

        <div className="mt-3 min-h-5" aria-live="polite">
          {message ? <p className="text-sm text-[var(--ok)]">{message}</p> : null}
          {!rotateOpen && rotationError ? <p role="alert" className="text-sm text-[var(--bad)]">{rotationError}</p> : null}
        </div>
      </Card>

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

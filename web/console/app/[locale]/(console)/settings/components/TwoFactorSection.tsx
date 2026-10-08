"use client";

import { useEffect, useRef, useState } from "react";
import { useAuth } from "../../../../contexts/AuthContext";
import { Button } from "../../../../components/ui/Button";
import { Input } from "../../../../components/ui/Input";
import { safeJSON, extractError } from "../../../../lib/api";

// ---------------------------------------------------------------------------
// Two-Factor Authentication Section
// ---------------------------------------------------------------------------

export type TwoFactorState = "idle" | "setting-up" | "verifying";

export function TwoFactorSection() {
  const { user, refreshUser } = useAuth();
  const totpEnabled = user?.totp_enabled === true;

  const [tfState, setTfState] = useState<TwoFactorState>("idle");
  const [setupSecret, setSetupSecret] = useState("");
  const [setupURI, setSetupURI] = useState("");
  const [verifyCode, setVerifyCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [disableCode, setDisableCode] = useState("");
  const [regenCode, setRegenCode] = useState("");
  const [showDisable, setShowDisable] = useState(false);
  const [showRegen, setShowRegen] = useState(false);
  const [tfLoading, setTfLoading] = useState(false);
  const [tfError, setTfError] = useState<string | null>(null);
  const [tfMessage, setTfMessage] = useState<string | null>(null);
  const [secretCopied, setSecretCopied] = useState(false);
  const [codesCopied, setCodesCopied] = useState(false);

  const resetTfStatus = () => {
    setTfError(null);
    setTfMessage(null);
  };

  const resetTfState = () => {
    setTfState("idle");
    setSetupSecret("");
    setSetupURI("");
    setVerifyCode("");
    setRecoveryCodes([]);
    setDisableCode("");
    setRegenCode("");
    setShowDisable(false);
    setShowRegen(false);
    setSecretCopied(false);
    setCodesCopied(false);
    resetTfStatus();
  };

  const handleStartSetup = async () => {
    resetTfStatus();
    setTfLoading(true);
    try {
      const response = await fetch("/api/auth/2fa/setup", { method: "POST" });
      const payload = await safeJSON(response);
      if (!response.ok) {
        setTfError(extractError(payload, "Failed to start 2FA setup"));
        return;
      }
      const data = payload as { secret?: string; uri?: string } | null;
      setSetupSecret(data?.secret ?? "");
      setSetupURI(data?.uri ?? "");
      setTfState("setting-up");
    } catch {
      setTfError("2FA setup endpoint unavailable");
    } finally {
      setTfLoading(false);
    }
  };

  const handleVerify = async () => {
    resetTfStatus();
    const code = verifyCode.trim();
    if (code.length !== 6 || !/^\d{6}$/.test(code)) {
      setTfError("Enter a valid 6-digit code");
      return;
    }

    setTfLoading(true);
    try {
      const response = await fetch("/api/auth/2fa/verify", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ code }),
      });
      const payload = await safeJSON(response);
      if (!response.ok) {
        setTfError(extractError(payload, "Verification failed"));
        return;
      }
      const data = payload as { recovery_codes?: string[] } | null;
      setRecoveryCodes(data?.recovery_codes ?? []);
      setTfState("verifying");
      setTfMessage("Two-factor authentication enabled successfully");
      await refreshUser();
    } catch {
      setTfError("2FA verify endpoint unavailable");
    } finally {
      setTfLoading(false);
    }
  };

  const handleDisable = async () => {
    resetTfStatus();
    const code = disableCode.trim();
    const isValidTOTP = /^\d{6}$/.test(code);
    const isValidRecovery = /^[0-9a-f]{8}-[0-9a-f]{8}$/i.test(code);
    if (!isValidTOTP && !isValidRecovery) {
      setTfError("Enter a 6-digit code or recovery code");
      return;
    }

    setTfLoading(true);
    try {
      const response = await fetch("/api/auth/2fa", {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ code }),
      });
      const payload = await safeJSON(response);
      if (!response.ok) {
        setTfError(extractError(payload, "Failed to disable 2FA"));
        return;
      }
      await refreshUser();
      resetTfState();
      setTfMessage("Two-factor authentication disabled");
    } catch {
      setTfError("2FA disable endpoint unavailable");
    } finally {
      setTfLoading(false);
    }
  };

  const handleRegenCodes = async () => {
    resetTfStatus();
    const code = regenCode.trim();
    const isValidTOTP = /^\d{6}$/.test(code);
    const isValidRecovery = /^[0-9a-f]{8}-[0-9a-f]{8}$/i.test(code);
    if (!isValidTOTP && !isValidRecovery) {
      setTfError("Enter a 6-digit code or recovery code");
      return;
    }

    setTfLoading(true);
    try {
      const response = await fetch("/api/auth/2fa/recovery-codes", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ code }),
      });
      const payload = await safeJSON(response);
      if (!response.ok) {
        setTfError(extractError(payload, "Failed to regenerate recovery codes"));
        return;
      }
      const data = payload as { recovery_codes?: string[] } | null;
      setRecoveryCodes(data?.recovery_codes ?? []);
      setShowRegen(false);
      setRegenCode("");
      setTfMessage("Recovery codes regenerated");
    } catch {
      setTfError("Recovery codes endpoint unavailable");
    } finally {
      setTfLoading(false);
    }
  };

  const copySecret = () => {
    void navigator.clipboard.writeText(setupSecret);
    setSecretCopied(true);
    setTimeout(() => setSecretCopied(false), 2000);
  };

  const copyRecoveryCodes = () => {
    void navigator.clipboard.writeText(recoveryCodes.join("\n"));
    setCodesCopied(true);
    setTimeout(() => setCodesCopied(false), 2000);
  };

  return (
    <div className="mt-6 pt-6 border-t border-[var(--line)]">
      <h3 className="text-sm font-medium text-[var(--text)]">Two-Factor Authentication</h3>

      {tfError ? <p className="mt-2 text-sm text-[var(--bad)]">{tfError}</p> : null}
      {tfMessage ? <p className="mt-2 text-sm text-[var(--muted)]">{tfMessage}</p> : null}

      {/* State: 2FA disabled, idle */}
      {!totpEnabled && tfState === "idle" ? (
        <div className="mt-2">
          <p className="text-sm text-[var(--muted)]">
            Two-factor authentication is not enabled. Add an extra layer of security to your account.
          </p>
          <Button variant="primary" className="mt-3" loading={tfLoading} onClick={() => void handleStartSetup()}>
            Enable 2FA
          </Button>
        </div>
      ) : null}

      {/* State: Setting up — show secret + QR + verify input */}
      {tfState === "setting-up" ? (
        <div className="mt-3 space-y-4">
          <p className="text-sm text-[var(--muted)]">
            Scan the QR code below with your authenticator app, or enter the secret key manually.
          </p>

          {setupURI ? (
            <div className="flex justify-center p-4 bg-white rounded-lg w-fit">
              <QRCodeCanvas data={setupURI} size={200} />
            </div>
          ) : null}

          <div>
            <p className="text-xs text-[var(--muted)] mb-1">Secret key (manual entry)</p>
            <div className="flex items-center gap-2">
              <code className="px-3 py-2 bg-[var(--surface)] border border-[var(--line)] rounded-lg text-sm font-mono text-[var(--text)] select-all break-all">
                {setupSecret}
              </code>
              <Button variant="ghost" size="sm" onClick={copySecret}>
                {secretCopied ? "Copied" : "Copy"}
              </Button>
            </div>
          </div>

          <div>
            <p className="text-xs text-[var(--muted)] mb-1">Enter the 6-digit code from your authenticator app</p>
            <div className="flex items-center gap-2">
              <Input
                value={verifyCode}
                onChange={(event) => setVerifyCode(event.target.value.replace(/\D/g, "").slice(0, 6))}
                placeholder="000000"
                maxLength={6}
                className="max-w-[160px] font-mono text-center tracking-widest"
                autoComplete="one-time-code"
              />
              <Button variant="primary" loading={tfLoading} onClick={() => void handleVerify()}>
                Verify &amp; Enable
              </Button>
              <Button variant="ghost" onClick={resetTfState}>
                Cancel
              </Button>
            </div>
          </div>
        </div>
      ) : null}

      {/* State: Just verified — show recovery codes */}
      {tfState === "verifying" && recoveryCodes.length > 0 ? (
        <div className="mt-3 space-y-3">
          <div className="p-3 border border-[var(--warn)]/40 bg-[var(--warn-glow)] rounded-lg">
            <p className="text-sm font-medium text-[var(--text)]">Save your recovery codes</p>
            <p className="text-xs text-[var(--muted)] mt-1">
              These codes can be used to access your account if you lose your authenticator device.
              Each code can only be used once. Store them in a safe place — they will not be shown again.
            </p>
          </div>

          <div className="grid grid-cols-2 gap-2 max-w-sm">
            {recoveryCodes.map((code) => (
              <code key={code} className="px-2 py-1 bg-[var(--surface)] border border-[var(--line)] rounded text-sm font-mono text-[var(--text)] text-center">
                {code}
              </code>
            ))}
          </div>

          <div className="flex items-center gap-2">
            <Button variant="secondary" size="sm" onClick={copyRecoveryCodes}>
              {codesCopied ? "Copied" : "Copy All"}
            </Button>
            <Button variant="ghost" size="sm" onClick={() => { setRecoveryCodes([]); resetTfState(); }}>
              Done
            </Button>
          </div>
        </div>
      ) : null}

      {/* State: 2FA enabled, idle */}
      {totpEnabled && tfState === "idle" ? (
        <div className="mt-2 space-y-3">
          <p className="text-sm text-[var(--ok)]">Two-factor authentication is enabled.</p>

          <div className="flex flex-wrap gap-2">
            {!showDisable ? (
              <Button variant="danger" size="sm" onClick={() => { setShowDisable(true); setShowRegen(false); resetTfStatus(); }}>
                Disable 2FA
              </Button>
            ) : null}
            {!showRegen ? (
              <Button variant="secondary" size="sm" onClick={() => { setShowRegen(true); setShowDisable(false); resetTfStatus(); }}>
                Regenerate Recovery Codes
              </Button>
            ) : null}
          </div>

          {showDisable ? (
            <div>
              <p className="text-xs text-[var(--muted)] mb-1">Enter your authenticator code or a recovery code to disable 2FA</p>
              <div className="flex items-center gap-2">
                <Input
                  value={disableCode}
                  onChange={(event) => setDisableCode(event.target.value.slice(0, 17))}
                  placeholder="Code or recovery code"
                  maxLength={17}
                  className="max-w-[240px] font-mono tracking-wide"
                  autoComplete="one-time-code"
                />
                <Button variant="danger" size="sm" loading={tfLoading} onClick={() => void handleDisable()}>
                  Confirm Disable
                </Button>
                <Button variant="ghost" size="sm" onClick={() => { setShowDisable(false); setDisableCode(""); resetTfStatus(); }}>
                  Cancel
                </Button>
              </div>
            </div>
          ) : null}

          {showRegen ? (
            <div>
              <p className="text-xs text-[var(--muted)] mb-1">Enter your authenticator code to regenerate recovery codes</p>
              <div className="flex items-center gap-2">
                <Input
                  value={regenCode}
                  onChange={(event) => setRegenCode(event.target.value.slice(0, 17))}
                  placeholder="Code or recovery code"
                  maxLength={17}
                  className="max-w-[240px] font-mono tracking-wide"
                  autoComplete="one-time-code"
                />
                <Button variant="primary" size="sm" loading={tfLoading} onClick={() => void handleRegenCodes()}>
                  Regenerate
                </Button>
                <Button variant="ghost" size="sm" onClick={() => { setShowRegen(false); setRegenCode(""); resetTfStatus(); }}>
                  Cancel
                </Button>
              </div>
            </div>
          ) : null}

          {/* Show regenerated recovery codes inline */}
          {recoveryCodes.length > 0 ? (
            <div className="space-y-3">
              <div className="p-3 border border-[var(--warn)]/40 bg-[var(--warn-glow)] rounded-lg">
                <p className="text-sm font-medium text-[var(--text)]">New recovery codes</p>
                <p className="text-xs text-[var(--muted)] mt-1">
                  Your previous recovery codes have been invalidated. Store these new codes in a safe place.
                </p>
              </div>

              <div className="grid grid-cols-2 gap-2 max-w-sm">
                {recoveryCodes.map((code) => (
                  <code key={code} className="px-2 py-1 bg-[var(--surface)] border border-[var(--line)] rounded text-sm font-mono text-[var(--text)] text-center">
                    {code}
                  </code>
                ))}
              </div>

              <Button variant="secondary" size="sm" onClick={copyRecoveryCodes}>
                {codesCopied ? "Copied" : "Copy All"}
              </Button>
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

// ---------------------------------------------------------------------------
// QR Code (client-side, no external service)
// ---------------------------------------------------------------------------

export function QRCodeCanvas({ data, size }: { data: string; size: number }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    if (!canvasRef.current || !data) return;
    let cancelled = false;
    import("qrcode")
      .then((QRCode) => {
        if (!cancelled && canvasRef.current) {
          return QRCode.toCanvas(canvasRef.current, data, {
            width: size,
            margin: 2,
            color: { dark: "#000000", light: "#ffffff" },
          });
        }
      })
      .catch(() => {
        // QR rendering failed — canvas remains blank.
      });
    return () => { cancelled = true; };
  }, [data, size]);

  return <canvas ref={canvasRef} />;
}

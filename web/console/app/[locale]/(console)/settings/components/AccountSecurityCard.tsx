"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";

import { useAuth } from "../../../../contexts/AuthContext";
import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { Input } from "../../../../components/ui/Input";
import { safeJSON, extractError } from "../../../../lib/api";

type TwoFactorState = "idle" | "setting-up" | "verifying";

// ── Password change section ──

function ChangePasswordSection() {
  const t = useTranslations("settings");

  const [open, setOpen] = useState(false);
  const [currentPw, setCurrentPw] = useState("");
  const [newPw, setNewPw] = useState("");
  const [confirmPw, setConfirmPw] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  const resetForm = () => {
    setCurrentPw("");
    setNewPw("");
    setConfirmPw("");
    setError(null);
    setMessage(null);
    setLoading(false);
  };

  const handleToggle = () => {
    setOpen((v) => !v);
    if (open) resetForm();
  };

  const handleSubmit = async () => {
    setError(null);
    setMessage(null);

    if (newPw.length < 8) {
      setError(t("security.passwordTooShort"));
      return;
    }
    if (newPw !== confirmPw) {
      setError(t("security.passwordMismatch"));
      return;
    }

    setLoading(true);
    try {
      const response = await fetch("/api/auth/me/password", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ current_password: currentPw, new_password: newPw }),
      });
      const payload = await safeJSON(response);
      if (!response.ok) {
        setError(extractError(payload, t("security.changePasswordFailed")));
        return;
      }
      setMessage(t("security.changePasswordSuccess"));
      resetForm();
      setOpen(false);
    } catch {
      setError(t("security.changePasswordUnavailable"));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="mt-4 pt-4 border-t border-[var(--line)]">
      <button
        type="button"
        className="flex items-center justify-between w-full text-left"
        onClick={handleToggle}
      >
        <h3 className="text-sm font-medium text-[var(--text)]">{t("security.changePassword")}</h3>
        <span className="text-xs text-[var(--muted)]">{open ? "▲" : "▼"}</span>
      </button>

      {message ? <p className="mt-2 text-sm text-[var(--ok)]">{message}</p> : null}

      {open ? (
        <div className="mt-3 space-y-3">
          {error ? <p className="text-sm text-[var(--bad)]">{error}</p> : null}
          <label className="block space-y-1">
            <span className="text-xs text-[var(--muted)]">{t("security.currentPassword")}</span>
            <Input
              type="password"
              value={currentPw}
              onChange={(e) => setCurrentPw(e.target.value)}
              autoComplete="current-password"
              disabled={loading}
              className="max-w-sm"
            />
          </label>
          <label className="block space-y-1">
            <span className="text-xs text-[var(--muted)]">{t("security.newPassword")}</span>
            <Input
              type="password"
              value={newPw}
              onChange={(e) => setNewPw(e.target.value)}
              autoComplete="new-password"
              disabled={loading}
              className="max-w-sm"
            />
          </label>
          <label className="block space-y-1">
            <span className="text-xs text-[var(--muted)]">{t("security.confirmPassword")}</span>
            <Input
              type="password"
              value={confirmPw}
              onChange={(e) => setConfirmPw(e.target.value)}
              autoComplete="new-password"
              disabled={loading}
              className="max-w-sm"
            />
          </label>
          <div className="flex items-center gap-2">
            <Button
              variant="primary"
              size="sm"
              loading={loading}
              onClick={() => void handleSubmit()}
            >
              {t("security.changePasswordSubmit")}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              disabled={loading}
              onClick={handleToggle}
            >
              {t("security.cancel")}
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

// ── Main card ──

export function AccountSecurityCard() {
  const t = useTranslations("settings");
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
        setTfError(extractError(payload, t("security.setupFailed")));
        return;
      }
      const data = payload as { secret?: string; uri?: string } | null;
      setSetupSecret(data?.secret ?? "");
      setSetupURI(data?.uri ?? "");
      setTfState("setting-up");
    } catch {
      setTfError(t("security.setupUnavailable"));
    } finally {
      setTfLoading(false);
    }
  };

  const handleVerify = async () => {
    resetTfStatus();
    const code = verifyCode.trim();
    if (code.length !== 6 || !/^\d{6}$/.test(code)) {
      setTfError(t("security.invalidCode"));
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
        setTfError(extractError(payload, t("security.verificationFailed")));
        return;
      }
      const data = payload as { recovery_codes?: string[] } | null;
      setRecoveryCodes(data?.recovery_codes ?? []);
      setTfState("verifying");
      setTfMessage(t("security.twoFactorEnabledSuccess"));
      await refreshUser();
    } catch {
      setTfError(t("security.verifyUnavailable"));
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
      setTfError(t("security.invalidCodeOrRecovery"));
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
        setTfError(extractError(payload, t("security.disableFailed")));
        return;
      }
      await refreshUser();
      resetTfState();
      setTfMessage(t("security.twoFactorDisabledSuccess"));
    } catch {
      setTfError(t("security.disableUnavailable"));
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
      setTfError(t("security.invalidCodeOrRecovery"));
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
        setTfError(extractError(payload, t("security.regenFailed")));
        return;
      }
      const data = payload as { recovery_codes?: string[] } | null;
      setRecoveryCodes(data?.recovery_codes ?? []);
      setShowRegen(false);
      setRegenCode("");
      setTfMessage(t("security.recoveryCodesRegenerated"));
    } catch {
      setTfError(t("security.recoveryCodesUnavailable"));
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
    <Card className="mb-6">
      <h2>{t("security.title")}</h2>

      <div className="mt-4 pt-4 border-t border-[var(--line)]">
        <h3 className="text-sm font-medium text-[var(--text)]">{t("security.twoFactor")}</h3>

        {tfError ? <p className="mt-2 text-sm text-[var(--bad)]">{tfError}</p> : null}
        {tfMessage ? <p className="mt-2 text-sm text-[var(--muted)]">{tfMessage}</p> : null}

        {!totpEnabled && tfState === "idle" ? (
          <div className="mt-2">
            <p className="text-sm text-[var(--muted)]">
              {t("security.twoFactorDisabled")}
            </p>
            <Button variant="primary" className="mt-3" loading={tfLoading} onClick={() => void handleStartSetup()}>
              {t("security.enableTwoFactor")}
            </Button>
          </div>
        ) : null}

        {tfState === "setting-up" ? (
          <div className="mt-3 space-y-4">
            <p className="text-sm text-[var(--muted)]">
              {t("security.scanQR")}
            </p>

            {setupURI ? (
              <div className="flex justify-center p-4 bg-white rounded-lg w-fit">
                <QRCodeCanvas data={setupURI} size={200} />
              </div>
            ) : null}

            <div>
              <p className="text-xs text-[var(--muted)] mb-1">{t("security.secretKey")}</p>
              <div className="flex items-center gap-2">
                <code className="px-3 py-2 bg-[var(--surface)] border border-[var(--line)] rounded-lg text-sm font-mono text-[var(--text)] select-all break-all">
                  {setupSecret}
                </code>
                <Button variant="ghost" size="sm" onClick={copySecret}>
                  {secretCopied ? t("security.copied") : t("security.copy")}
                </Button>
              </div>
            </div>

            <div>
              <p className="text-xs text-[var(--muted)] mb-1">{t("security.enterCode")}</p>
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
                  {t("security.verifyEnable")}
                </Button>
                <Button variant="ghost" onClick={resetTfState}>
                  {t("security.cancel")}
                </Button>
              </div>
            </div>
          </div>
        ) : null}

        {tfState === "verifying" && recoveryCodes.length > 0 ? (
          <div className="mt-3 space-y-3">
            <div className="p-3 border border-[var(--warn)]/40 bg-[var(--warn-glow)] rounded-lg">
              <p className="text-sm font-medium text-[var(--text)]">{t("security.saveRecoveryCodes")}</p>
              <p className="text-xs text-[var(--muted)] mt-1">
                {t("security.recoveryCodesDescription")}
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
                {codesCopied ? t("security.copied") : t("security.copyAll")}
              </Button>
              <Button variant="ghost" size="sm" onClick={() => { setRecoveryCodes([]); resetTfState(); }}>
                {t("security.done")}
              </Button>
            </div>
          </div>
        ) : null}

        {totpEnabled && tfState === "idle" ? (
          <div className="mt-2 space-y-3">
            <p className="text-sm text-[var(--ok)]">{t("security.twoFactorEnabled")}</p>

            <div className="flex flex-wrap gap-2">
              {!showDisable ? (
                <Button variant="danger" size="sm" onClick={() => { setShowDisable(true); setShowRegen(false); resetTfStatus(); }}>
                  {t("security.disable2FA")}
                </Button>
              ) : null}
              {!showRegen ? (
                <Button variant="secondary" size="sm" onClick={() => { setShowRegen(true); setShowDisable(false); resetTfStatus(); }}>
                  {t("security.regenerateRecoveryCodes")}
                </Button>
              ) : null}
            </div>

            {showDisable ? (
              <div>
                <p className="text-xs text-[var(--muted)] mb-1">{t("security.enterCodeToDisable")}</p>
                <div className="flex items-center gap-2">
                  <Input
                    value={disableCode}
                    onChange={(event) => setDisableCode(event.target.value.slice(0, 17))}
                    placeholder={t("security.codeOrRecoveryPlaceholder")}
                    maxLength={17}
                    className="max-w-[240px] font-mono tracking-wide"
                    autoComplete="one-time-code"
                  />
                  <Button variant="danger" size="sm" loading={tfLoading} onClick={() => void handleDisable()}>
                    {t("security.confirmDisable")}
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => { setShowDisable(false); setDisableCode(""); resetTfStatus(); }}>
                    {t("security.cancel")}
                  </Button>
                </div>
              </div>
            ) : null}

            {showRegen ? (
              <div>
                <p className="text-xs text-[var(--muted)] mb-1">{t("security.enterCodeToRegen")}</p>
                <div className="flex items-center gap-2">
                  <Input
                    value={regenCode}
                    onChange={(event) => setRegenCode(event.target.value.slice(0, 17))}
                    placeholder={t("security.codeOrRecoveryPlaceholder")}
                    maxLength={17}
                    className="max-w-[240px] font-mono tracking-wide"
                    autoComplete="one-time-code"
                  />
                  <Button variant="primary" size="sm" loading={tfLoading} onClick={() => void handleRegenCodes()}>
                    {t("security.regenerate")}
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => { setShowRegen(false); setRegenCode(""); resetTfStatus(); }}>
                    {t("security.cancel")}
                  </Button>
                </div>
              </div>
            ) : null}

            {recoveryCodes.length > 0 ? (
              <div className="space-y-3">
                <div className="p-3 border border-[var(--warn)]/40 bg-[var(--warn-glow)] rounded-lg">
                  <p className="text-sm font-medium text-[var(--text)]">{t("security.newRecoveryCodes")}</p>
                  <p className="text-xs text-[var(--muted)] mt-1">
                    {t("security.newRecoveryCodesDescription")}
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
                  {codesCopied ? t("security.copied") : t("security.copyAll")}
                </Button>
              </div>
            ) : null}
          </div>
        ) : null}
      </div>

      <ChangePasswordSection />
    </Card>
  );
}

function QRCodeCanvas({ data, size }: { data: string; size: number }) {
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


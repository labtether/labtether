"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "../../../i18n/navigation";
import { useTranslations } from "next-intl";
import { Card } from "../../components/ui/Card";
import { Button } from "../../components/ui/Button";
import { Input } from "../../components/ui/Input";
import { useTailscaleServeStatus } from "../../hooks/useTailscaleServeStatus";
import { runtimeSettingKeys } from "../../console/models";
import { safeLocalRedirectPath } from "../../../lib/safeRedirect";
import { buildLocalSetupDestination, buildRemoteAccessURL } from "./setupNavigation";
import { SetupRemoteAccessStep } from "./SetupRemoteAccessStep";

export default function SetupPage() {
  const router = useRouter();
  const t = useTranslations("auth");
  const [step, setStep] = useState<"account" | "remote-access">(() => {
    if (typeof window === "undefined") return "account";
    return new URLSearchParams(window.location.search).get("step") ===
      "remote-access"
      ? "remote-access"
      : "account";
  });
  const [username, setUsername] = useState("");
  const [setupToken, setSetupToken] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [nextPath, setNextPath] = useState(() => {
    if (typeof window === "undefined") return "/";
    return safeLocalRedirectPath(
      new URLSearchParams(window.location.search).get("next"),
    );
  });
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [ready, setReady] = useState(false);
  const [copied, setCopied] = useState("");
  const [managedActionLoading, setManagedActionLoading] = useState<
    "" | "apply" | "disable"
  >("");
  const [managedActionMessage, setManagedActionMessage] = useState("");
  const [managedActionError, setManagedActionError] = useState("");
  const submitAbortRef = useRef<AbortController | null>(null);
  const {
    status: tailscaleStatus,
    loading: tailscaleLoading,
    error: tailscaleError,
    refresh: refreshTailscaleStatus,
  } = useTailscaleServeStatus(step === "remote-access");

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    setNextPath(safeLocalRedirectPath(params.get("next")));
    setStep(
      params.get("step") === "remote-access" ? "remote-access" : "account",
    );
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function loadStatus() {
      try {
        const response = await fetch("/api/auth/bootstrap/status", {
          cache: "no-store",
        });
        const payload = await response.json().catch(() => null);
        if (cancelled) return;
        if (!response.ok) {
          setError(payload?.error ?? t("setup.loadError"));
          setReady(true);
          return;
        }
        if (!payload?.setup_required) {
          if (step === "remote-access") {
            setReady(true);
            return;
          }
          router.replace(`/login?next=${encodeURIComponent(nextPath)}`);
          return;
        }
        if (
          typeof payload?.suggested_username === "string" &&
          payload.suggested_username.trim() !== ""
        ) {
          setUsername(payload.suggested_username.trim());
        }
      } catch {
        if (!cancelled) {
          setError(t("setup.connectionError"));
        }
      } finally {
        if (!cancelled) {
          setReady(true);
        }
      }
    }

    void loadStatus();
    return () => {
      cancelled = true;
    };
  }, [t, nextPath, router, step]);

  useEffect(() => {
    return () => {
      submitAbortRef.current?.abort();
      submitAbortRef.current = null;
    };
  }, []);

  const passwordMismatch = useMemo(
    () => confirmPassword !== "" && password !== confirmPassword,
    [confirmPassword, password],
  );

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (passwordMismatch) {
      setError(t("setup.passwordMismatch"));
      return;
    }

    setError(null);
    setLoading(true);
    submitAbortRef.current?.abort();
    const controller = new AbortController();
    submitAbortRef.current = controller;

    try {
      const response = await fetch("/api/auth/bootstrap", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ setup_token: setupToken, username, password }),
        credentials: "same-origin",
        signal: controller.signal,
      });
      const payload = await response.json().catch(() => null);
      if (!response.ok) {
        setSetupToken("");
        setPassword("");
        setConfirmPassword("");
        setError(payload?.error ?? t("setup.failed"));
        return;
      }
      setSetupToken("");
      setPassword("");
      setConfirmPassword("");
      setStep("remote-access");
      if (typeof window !== "undefined") {
        const params = new URLSearchParams();
        if (nextPath !== "/") {
          params.set("next", nextPath);
        }
        params.set("step", "remote-access");
        const query = params.toString();
        window.history.replaceState(
          {},
          "",
          query ? `/setup?${query}` : "/setup",
        );
      }
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") {
        return;
      }
      setSetupToken("");
      setPassword("");
      setConfirmPassword("");
      setError(t("setup.connectionError"));
    } finally {
      setLoading(false);
    }
  };

  const copyToClipboard = (text: string, label: string) => {
    void navigator.clipboard.writeText(text);
    setCopied(label);
    setTimeout(() => setCopied(""), 2000);
  };

  const persistRemoteAccessMode = async (mode: "serve" | "off") => {
    try {
      await fetch("/api/settings/runtime", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          values: {
            [runtimeSettingKeys.remoteAccessMode]: mode,
          },
        }),
      });
    } catch {
      // Best effort only. Setup should remain non-blocking even if the preference save fails.
    }
  };

  const continueToConsole = async (mode: "serve" | "off") => {
    await persistRemoteAccessMode(mode);
    if (
      mode === "serve" &&
      tailscaleStatus?.serve_configured &&
      typeof tailscaleStatus.tsnet_url === "string" &&
      tailscaleStatus.tsnet_url.trim() !== ""
    ) {
      window.location.assign(
        buildRemoteAccessURL(tailscaleStatus.tsnet_url, nextPath),
      );
      return;
    }
    // next-intl's router always prefixes the active locale. `nextPath` is
    // already the exact localized return path (for example `/en`), so passing
    // it through that router produces `/en/en`. Use an exact same-origin
    // navigation at the one-time setup boundary instead.
    window.location.assign(buildLocalSetupDestination(nextPath));
  };

  const runManagedTailscaleAction = async (action: "apply" | "disable") => {
    setManagedActionLoading(action);
    setManagedActionMessage("");
    setManagedActionError("");
    try {
      const response = await fetch("/api/settings/tailscale/serve", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action }),
      });
      const payload = (await response.json().catch(() => null)) as {
        error?: string;
      } | null;
      if (!response.ok) {
        throw new Error(payload?.error || `HTTP ${response.status}`);
      }
      setManagedActionMessage(
        action === "apply"
          ? t("remoteAccess.enabledMessage")
          : t("remoteAccess.disabledMessage"),
      );
      refreshTailscaleStatus();
    } catch (err: unknown) {
      setManagedActionError(
        err instanceof Error ? err.message : t("remoteAccess.managedError"),
      );
    } finally {
      setManagedActionLoading("");
    }
  };

  if (!ready) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-[var(--bg)]">
        <p className="text-sm text-[var(--muted)]">{t("setup.preparing")}</p>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-[var(--bg)] relative overflow-hidden">
      <div
        className="absolute w-[400px] h-[400px] rounded-full pointer-events-none opacity-20 blur-[120px]"
        style={{ background: "var(--accent)", top: "-10%", right: "-5%" }}
      />
      <div
        className="absolute w-[300px] h-[300px] rounded-full pointer-events-none opacity-10 blur-[100px]"
        style={{
          background: "var(--accent-secondary)",
          bottom: "-10%",
          left: "-5%",
        }}
      />
      <div
        className="absolute inset-0 pointer-events-none opacity-[0.03]"
        style={{
          backgroundImage:
            "radial-gradient(circle, var(--text) 1px, transparent 1px)",
          backgroundSize: "24px 24px",
        }}
      />

      <div
        className={`relative w-full z-10 ${step === "remote-access" ? "max-w-4xl px-4" : "max-w-sm"}`}
      >
        <div className="absolute -inset-px rounded-xl overflow-hidden pointer-events-none">
          <div
            className="absolute inset-0"
            style={{
              background:
                "conic-gradient(from 0deg, transparent 0%, var(--accent) 10%, transparent 20%, transparent 50%, var(--accent-secondary) 60%, transparent 70%)",
              animation: "border-rotate 8s linear infinite",
              opacity: 0.4,
            }}
          />
        </div>

        <Card className="relative space-y-6">
          <div
            className="absolute top-0 left-[10%] right-[10%] h-px pointer-events-none"
            style={{
              background:
                "linear-gradient(90deg, transparent, color-mix(in srgb, var(--accent) 40%, white), transparent)",
            }}
          />

          <div className="flex flex-col items-center gap-3">
            <img src="/logo.svg" alt="LabTether" width={56} height={56} />
            <div className="text-center">
              <h1 className="text-xl font-medium text-[var(--text)] font-[family-name:var(--font-heading)]">
                {step === "account"
                  ? t("setup.title")
                  : t("remoteAccess.title")}
              </h1>
              <p className="mt-0.5 text-[10px] font-mono uppercase tracking-[0.15em] text-[var(--muted)]">
                {step === "account"
                  ? t("setup.subtitle")
                  : t("remoteAccess.subtitle")}
              </p>
            </div>
          </div>

          {step === "account" ? (
            <form onSubmit={handleSubmit} className="space-y-4">
              {error ? (
                <div
                  role="alert"
                  className="rounded-lg bg-[var(--bad-glow)] px-3 py-2 text-sm text-[var(--bad)]"
                >
                  {error}
                </div>
              ) : null}

              <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)]">
                {t("setup.setupToken")}
                <Input
                  type="password"
                  value={setupToken}
                  onChange={(e) => setSetupToken(e.target.value)}
                  autoFocus
                  autoComplete="off"
                  placeholder={t("setup.setupTokenPlaceholder")}
                  required
                  maxLength={512}
                />
                <span className="text-[11px] leading-relaxed">
                  {t("setup.setupTokenHelp")}
                </span>
              </label>

              <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)]">
                {t("setup.username")}
                <Input
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  autoComplete="username"
                  placeholder="owner"
                  required
                />
              </label>

              <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)]">
                {t("setup.password")}
                <Input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="new-password"
                  placeholder={t("setup.passwordPlaceholder")}
                  required
                />
              </label>

              <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)]">
                {t("setup.confirmPassword")}
                <Input
                  type="password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  autoComplete="new-password"
                  placeholder={t("setup.confirmPasswordPlaceholder")}
                  required
                />
              </label>

              <Button
                type="submit"
                variant="primary"
                className="w-full"
                disabled={
                  loading || passwordMismatch || setupToken.trim() === ""
                }
              >
                {loading ? t("setup.submitting") : t("setup.submit")}
              </Button>
            </form>
          ) : (
            <SetupRemoteAccessStep
              tailscaleStatus={tailscaleStatus}
              tailscaleLoading={tailscaleLoading}
              tailscaleError={tailscaleError}
              refreshTailscaleStatus={refreshTailscaleStatus}
              copied={copied}
              copyToClipboard={copyToClipboard}
              managedActionLoading={managedActionLoading}
              managedActionMessage={managedActionMessage}
              managedActionError={managedActionError}
              runManagedTailscaleAction={runManagedTailscaleAction}
              continueToConsole={continueToConsole}
            />
          )}
        </Card>
      </div>
    </div>
  );
}

"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "../../../i18n/navigation";
import { useTranslations } from "next-intl";
import { CheckCircle2, Copy, RefreshCw, Shield, SkipForward } from "lucide-react";
import { Card } from "../../components/ui/Card";
import { Button } from "../../components/ui/Button";
import { Input } from "../../components/ui/Input";
import { Badge } from "../../components/ui/Badge";
import { useTailscaleServeStatus } from "../../hooks/useTailscaleServeStatus";
import { runtimeSettingKeys } from "../../console/models";

function safeNextPath(value: string | null): string {
  if (!value || !value.startsWith("/")) return "/";
  if (value.startsWith("//")) return "/";
  const lower = value.toLowerCase();
  if (lower.includes("javascript:") || lower.includes("data:")) return "/";
  return value;
}

function buildRemoteAccessURL(baseURL: string, nextPath: string): string {
  try {
    const target = new URL(baseURL);
    target.pathname = nextPath || "/";
    target.search = "";
    target.hash = "";
    return target.toString();
  } catch {
    return baseURL;
  }
}

export default function SetupPage() {
  const router = useRouter();
  const t = useTranslations('auth');
  const [step, setStep] = useState<"account" | "remote-access">(() => {
    if (typeof window === "undefined") return "account";
    return new URLSearchParams(window.location.search).get("step") === "remote-access" ? "remote-access" : "account";
  });
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [nextPath, setNextPath] = useState(() => {
    if (typeof window === "undefined") return "/";
    return safeNextPath(new URLSearchParams(window.location.search).get("next"));
  });
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [ready, setReady] = useState(false);
  const [copied, setCopied] = useState("");
  const [managedActionLoading, setManagedActionLoading] = useState<"" | "apply" | "disable">("");
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
    setNextPath(safeNextPath(params.get("next")));
    setStep(params.get("step") === "remote-access" ? "remote-access" : "account");
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function loadStatus() {
      try {
        const response = await fetch("/api/auth/bootstrap/status", { cache: "no-store" });
        const payload = await response.json().catch(() => null);
        if (cancelled) return;
        if (!response.ok) {
          setError(payload?.error ?? t('setup.loadError'));
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
        if (typeof payload?.suggested_username === "string" && payload.suggested_username.trim() !== "") {
          setUsername(payload.suggested_username.trim());
        }
      } catch {
        if (!cancelled) {
          setError(t('setup.connectionError'));
        }
      } finally {
        if (!cancelled) {
          setReady(true);
        }
      }
    }

    void loadStatus();
    return () => { cancelled = true; };
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
      setError(t('setup.passwordMismatch'));
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
        body: JSON.stringify({ username, password }),
        credentials: "same-origin",
        signal: controller.signal,
      });
      const payload = await response.json().catch(() => null);
      if (!response.ok) {
        setError(payload?.error ?? t('setup.failed'));
        return;
      }
      setStep("remote-access");
      if (typeof window !== "undefined") {
        const params = new URLSearchParams();
        if (nextPath !== "/") {
          params.set("next", nextPath);
        }
        params.set("step", "remote-access");
        const query = params.toString();
        window.history.replaceState({}, "", query ? `/setup?${query}` : "/setup");
      }
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") {
        return;
      }
      setError(t('setup.connectionError'));
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
      window.location.assign(buildRemoteAccessURL(tailscaleStatus.tsnet_url, nextPath));
      return;
    }
    router.push(nextPath);
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
      const payload = await response.json().catch(() => null) as { error?: string } | null;
      if (!response.ok) {
        throw new Error(payload?.error || `HTTP ${response.status}`);
      }
      setManagedActionMessage(action === "apply" ? t('remoteAccess.enabledMessage') : t('remoteAccess.disabledMessage'));
      refreshTailscaleStatus();
    } catch (err: unknown) {
      setManagedActionError(err instanceof Error ? err.message : t('remoteAccess.managedError'));
    } finally {
      setManagedActionLoading("");
    }
  };

  if (!ready) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-[var(--bg)]">
        <p className="text-sm text-[var(--muted)]">{t('setup.preparing')}</p>
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
        style={{ background: "var(--accent-secondary)", bottom: "-10%", left: "-5%" }}
      />
      <div
        className="absolute inset-0 pointer-events-none opacity-[0.03]"
        style={{
          backgroundImage: "radial-gradient(circle, var(--text) 1px, transparent 1px)",
          backgroundSize: "24px 24px",
        }}
      />

      <div className={`relative w-full z-10 ${step === "remote-access" ? "max-w-4xl px-4" : "max-w-sm"}`}>
        <div className="absolute -inset-px rounded-xl overflow-hidden pointer-events-none">
          <div
            className="absolute inset-0"
            style={{
              background: "conic-gradient(from 0deg, transparent 0%, var(--accent) 10%, transparent 20%, transparent 50%, var(--accent-secondary) 60%, transparent 70%)",
              animation: "border-rotate 8s linear infinite",
              opacity: 0.4,
            }}
          />
        </div>

        <Card className="relative space-y-6">
          <div
            className="absolute top-0 left-[10%] right-[10%] h-px pointer-events-none"
            style={{ background: "linear-gradient(90deg, transparent, color-mix(in srgb, var(--accent) 40%, white), transparent)" }}
          />

          <div className="flex flex-col items-center gap-3">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src="/logo.svg" alt="LabTether" width={48} height={48} />
            <div className="text-center">
              <h1 className="text-xl font-medium text-[var(--text)] font-[family-name:var(--font-heading)]">
                {step === "account" ? t('setup.title') : t('remoteAccess.title')}
              </h1>
              <p className="mt-0.5 text-[10px] font-mono uppercase tracking-[0.15em] text-[var(--muted)]">
                {step === "account" ? t('setup.subtitle') : t('remoteAccess.subtitle')}
              </p>
            </div>
          </div>

          {step === "account" ? (
            <form onSubmit={handleSubmit} className="space-y-4">
              {error ? <div className="rounded-lg bg-[var(--bad-glow)] px-3 py-2 text-sm text-[var(--bad)]">{error}</div> : null}

              <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)]">
                {t('setup.username')}
                <Input
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  autoFocus
                  autoComplete="username"
                  placeholder="owner"
                  required
                />
              </label>

              <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)]">
                {t('setup.password')}
                <Input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="new-password"
                  placeholder={t('setup.passwordPlaceholder')}
                  required
                />
              </label>

              <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)]">
                {t('setup.confirmPassword')}
                <Input
                  type="password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  autoComplete="new-password"
                  placeholder={t('setup.confirmPasswordPlaceholder')}
                  required
                />
              </label>

              <Button type="submit" variant="primary" className="w-full" disabled={loading || passwordMismatch}>
                {loading ? t('setup.submitting') : t('setup.submit')}
              </Button>
            </form>
          ) : (
            <div className="space-y-6">
              <div className="rounded-lg border border-[var(--accent)]/20 bg-[var(--accent-glow)]/30 px-4 py-3">
                <div className="flex items-start gap-3">
                  <Shield size={16} className="mt-0.5 shrink-0 text-[var(--accent)]" />
                  <div className="space-y-1.5">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge status={tailscaleStatus?.serve_configured ? "enabled" : "pending"} size="sm" />
                      <span className="text-sm font-medium text-[var(--text)]">{t('remoteAccess.recommended')}</span>
                    </div>
                    <p className="text-sm text-[var(--text)]/90">
                      {t('remoteAccess.description')}
                    </p>
                    <p className="text-xs text-[var(--muted)]">
                      {t('remoteAccess.skipNote')}
                    </p>
                  </div>
                </div>
              </div>

              <div className="grid gap-6 lg:grid-cols-[minmax(0,1.15fr)_minmax(18rem,0.85fr)]">
                <div className="space-y-4">
                  <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)]/55 p-4">
                    <p className="text-[10px] font-mono uppercase tracking-[0.14em] text-[var(--muted)]">{t('remoteAccess.suggestedSteps')}</p>
                    <div className="mt-3 space-y-3 text-sm text-[var(--text)]">
                      <div>
                        <p className="font-medium">{t('remoteAccess.step1.title')}</p>
                        <p className="text-xs text-[var(--muted)]">{t('remoteAccess.step1.description')}</p>
                      </div>
                      <div>
                        <p className="font-medium">{t('remoteAccess.step2.title')}</p>
                        {tailscaleStatus?.suggested_command ? (
                          <div className="mt-2 rounded-lg border border-[var(--line)] bg-[var(--bg)]/70 p-3">
                            <code className="block break-all text-xs sm:text-sm">{tailscaleStatus.suggested_command}</code>
                            <div className="mt-3 flex flex-wrap gap-2">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => copyToClipboard(tailscaleStatus.suggested_command!, "setup-command")}
                              >
                                <Copy size={13} className="shrink-0" />
                                {copied === "setup-command" ? t('remoteAccess.copied') : t('remoteAccess.copyCommand')}
                              </Button>
                            </div>
                          </div>
                        ) : (
                          <p className="mt-1 text-xs text-[var(--muted)]">{t('remoteAccess.step2.fallback')}</p>
                        )}
                      </div>
                      <div>
                        <p className="font-medium">{t('remoteAccess.step3.title')}</p>
                        <p className="text-xs text-[var(--muted)]">{t('remoteAccess.step3.description')}</p>
                      </div>
                    </div>
                  </div>

                  <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)]/55 p-4">
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <div>
                        <p className="text-[10px] font-mono uppercase tracking-[0.14em] text-[var(--muted)]">{t('remoteAccess.detectedStatus')}</p>
                        <p className="mt-1 text-sm text-[var(--text)]">{t('remoteAccess.detectedDescription')}</p>
                      </div>
                      <Button variant="ghost" size="sm" loading={tailscaleLoading && Boolean(tailscaleStatus)} onClick={refreshTailscaleStatus}>
                        <RefreshCw size={13} className="shrink-0" />
                        {t('remoteAccess.verify')}
                      </Button>
                    </div>

                    {tailscaleError && !tailscaleStatus ? (
                      <div className="mt-3 rounded-lg bg-[var(--bad-glow)] px-3 py-2 text-sm text-[var(--bad)]">{tailscaleError}</div>
                    ) : null}

                    <div className="mt-4 grid gap-3 sm:grid-cols-2">
                      <div className="rounded-lg border border-[var(--line)] bg-[var(--bg)]/60 px-3 py-3">
                        <p className="text-[10px] font-mono uppercase tracking-[0.12em] text-[var(--muted)]">{t('remoteAccess.tailscale')}</p>
                        <div className="mt-2 flex items-center gap-2">
                          <Badge status={tailscaleStatus?.tailscale_installed ? (tailscaleStatus.logged_in ? "enabled" : "pending") : "disabled"} size="sm" />
                          <span className="text-sm text-[var(--text)]">
                            {tailscaleStatus
                              ? tailscaleStatus.tailscale_installed
                                ? tailscaleStatus.backend_state || (tailscaleStatus.logged_in ? t('remoteAccess.connected') : t('remoteAccess.installed'))
                                : t('remoteAccess.notDetected')
                              : tailscaleLoading
                                ? t('remoteAccess.checking')
                                : t('remoteAccess.unavailable')}
                          </span>
                        </div>
                      </div>

                      <div className="rounded-lg border border-[var(--line)] bg-[var(--bg)]/60 px-3 py-3">
                        <p className="text-[10px] font-mono uppercase tracking-[0.12em] text-[var(--muted)]">{t('remoteAccess.serve')}</p>
                        <div className="mt-2 flex items-center gap-2">
                          <Badge status={tailscaleStatus?.serve_configured ? "enabled" : tailscaleStatus?.logged_in ? "pending" : "disabled"} size="sm" />
                          <span className="text-sm text-[var(--text)]">
                            {tailscaleStatus
                              ? tailscaleStatus.serve_configured
                                ? t('remoteAccess.httpsActive')
                                : tailscaleStatus.serve_status === "login_required"
                                  ? t('remoteAccess.loginRequired')
                                  : tailscaleStatus.serve_status === "not_installed"
                                    ? t('remoteAccess.notInstalled')
                                    : t('remoteAccess.notConfigured')
                              : tailscaleLoading
                                ? t('remoteAccess.checking')
                                : t('remoteAccess.unavailable')}
                          </span>
                        </div>
                      </div>
                    </div>

                    {tailscaleStatus?.tsnet_url ? (
                      <div className="mt-3 rounded-lg border border-[var(--line)] bg-[var(--bg)]/60 px-3 py-3">
                        <p className="text-[10px] font-mono uppercase tracking-[0.12em] text-[var(--muted)]">{t('remoteAccess.httpsUrl')}</p>
                        <code className="mt-2 block break-all text-xs sm:text-sm">{tailscaleStatus.tsnet_url}</code>
                        <div className="mt-3">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => copyToClipboard(tailscaleStatus.tsnet_url!, "setup-url")}
                          >
                            <Copy size={13} className="shrink-0" />
                            {copied === "setup-url" ? t('remoteAccess.copied') : t('remoteAccess.copyUrl')}
                          </Button>
                        </div>
                      </div>
                    ) : null}

                    {tailscaleStatus?.status_note ? (
                      <p className="mt-3 text-xs text-[var(--muted)]">{tailscaleStatus.status_note}</p>
                    ) : null}

                    {tailscaleStatus?.can_manage ? (
                      <div className="mt-4 rounded-lg border border-[var(--accent)]/20 bg-[var(--accent-glow)]/15 px-3 py-3">
                        <p className="text-sm font-medium text-[var(--text)]">{t('remoteAccess.managedTitle')}</p>
                        <p className="mt-1 text-xs text-[var(--muted)]">
                          {t('remoteAccess.managedDescription')}
                        </p>
                        <div className="mt-3 flex flex-wrap gap-2">
                          <Button
                            variant="primary"
                            size="sm"
                            loading={managedActionLoading === "apply"}
                            onClick={() => void runManagedTailscaleAction("apply")}
                          >
                            {tailscaleStatus.serve_configured ? t('remoteAccess.reapplyHttps') : t('remoteAccess.enableHttps')}
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            loading={managedActionLoading === "disable"}
                            onClick={() => void runManagedTailscaleAction("disable")}
                          >
                            {t('remoteAccess.disableHttps')}
                          </Button>
                        </div>
                        {managedActionMessage ? <p className="mt-3 text-xs text-[var(--muted)]">{managedActionMessage}</p> : null}
                        {managedActionError ? <p className="mt-3 text-xs text-[var(--bad)]">{managedActionError}</p> : null}
                      </div>
                    ) : null}
                  </div>
                </div>

                <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)]/55 p-4">
                  <p className="text-[10px] font-mono uppercase tracking-[0.14em] text-[var(--muted)]">{t('remoteAccess.finishSetup')}</p>
                  <div className="mt-3 space-y-3">
                    <div className="rounded-lg border border-[var(--line)] bg-[var(--bg)]/60 px-3 py-3">
                      <p className="text-sm font-medium text-[var(--text)]">{t('remoteAccess.continueNow')}</p>
                      <p className="mt-1 text-xs text-[var(--muted)]">
                        {t('remoteAccess.continueNowDescription')}
                      </p>
                      <Button variant="secondary" className="mt-3 w-full" onClick={() => void continueToConsole("off")}>
                        <SkipForward size={14} className="shrink-0" />
                        {t('remoteAccess.continueWithout')}
                      </Button>
                    </div>

                    <div className="rounded-lg border border-[var(--accent)]/20 bg-[var(--accent-glow)]/20 px-3 py-3">
                      <p className="text-sm font-medium text-[var(--text)]">{t('remoteAccess.finishRecommended')}</p>
                      <p className="mt-1 text-xs text-[var(--muted)]">
                        {t('remoteAccess.finishRecommendedDescription')}
                      </p>
                      <Button variant="primary" className="mt-3 w-full" onClick={() => void continueToConsole("serve")}>
                        <CheckCircle2 size={14} className="shrink-0" />
                        {t('remoteAccess.openLabTether')}
                      </Button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          )}
        </Card>
      </div>
    </div>
  );
}

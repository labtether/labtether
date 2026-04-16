"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "../../../i18n/navigation";
import { useTranslations } from "next-intl";
import { Card } from "../../components/ui/Card";
import { Button } from "../../components/ui/Button";
import { Input } from "../../components/ui/Input";

function safeNextPath(value: string | null): string {
  if (!value || !value.startsWith("/")) return "/";
  // Prevent protocol-relative/open redirects.
  if (value.startsWith("//")) return "/";
  // Block javascript: and data: URIs that could bypass the leading "/" check via encoding.
  const lower = value.toLowerCase();
  if (lower.includes("javascript:") || lower.includes("data:")) return "/";
  return value;
}

export default function LoginPage() {
  const router = useRouter();
  const t = useTranslations('auth');
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [nextPath, setNextPath] = useState(() => {
    if (typeof window === "undefined") return "/";
    return safeNextPath(new URLSearchParams(window.location.search).get("next"));
  });
  const [error, setError] = useState<string | null>(null);
  const [oidcEnabled, setOIDCEnabled] = useState(false);
  const [oidcProviderName, setOidcProviderName] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [challengeToken, setChallengeToken] = useState<string | null>(null);
  const [totpCode, setTotpCode] = useState("");
  const submitAbortRef = useRef<AbortController | null>(null);
  const submitSeqRef = useRef(0);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    setNextPath(safeNextPath(params.get("next")));
    const errorParam = params.get("error");
    if (errorParam) {
      setError(errorParam);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    async function checkBootstrapStatus() {
      try {
        const response = await fetch("/api/auth/bootstrap/status", { cache: "no-store" });
        if (!response.ok) return;
        const payload = await response.json().catch(() => null);
        if (!cancelled && payload?.setup_required) {
          router.replace(`/setup?next=${encodeURIComponent(nextPath)}`);
        }
      } catch {
        // Keep login available when bootstrap status is unavailable.
      }
    }
    void checkBootstrapStatus();
    return () => { cancelled = true; };
  }, [nextPath, router]);

  useEffect(() => {
    return () => {
      submitAbortRef.current?.abort();
      submitAbortRef.current = null;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    async function loadProviders() {
      try {
        const response = await fetch("/api/auth/providers", { cache: "no-store" });
        if (!response.ok) return;
        const payload = await response.json();
        if (cancelled) return;
        const oidc = payload?.oidc;
        if (oidc?.enabled) {
          setOIDCEnabled(true);
          if (typeof oidc.display_name === "string" && oidc.display_name.trim() !== "") {
            setOidcProviderName(oidc.display_name.trim());
          }
        }
      } catch {
        // Keep password-only flow when provider metadata is unavailable.
      }
    }
    void loadProviders();
    return () => { cancelled = true; };
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setLoading(true);
    const requestID = ++submitSeqRef.current;
    submitAbortRef.current?.abort();
    const controller = new AbortController();
    submitAbortRef.current = controller;

    try {
      const response = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password }),
        credentials: "same-origin",
        signal: controller.signal,
      });

      const data = await response.json().catch(() => null);
      if (response.ok && data?.requires_2fa) {
        setChallengeToken(data.challenge_token);
        setLoading(false);
        return;
      }

      if (!response.ok) {
        setError(data?.error ?? t('login.failed'));
        return;
      }

      router.push(nextPath);
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") {
        return;
      }
      setError(t('login.connectionError'));
    } finally {
      if (requestID === submitSeqRef.current) {
        setLoading(false);
      }
    }
  };

  const handleOIDCLogin = () => {
    window.location.href = `/api/auth/oidc/start?next=${encodeURIComponent(nextPath)}`;
  };

  const handleSubmit2FA = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      const response = await fetch("/api/auth/login/2fa", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ challenge_token: challengeToken, code: totpCode }),
        credentials: "same-origin",
      });
      if (!response.ok) {
        const data = await response.json().catch(() => null);
        // The server consumes the challenge token on the first attempt. A retry
        // with the same token will always fail. Clear and send user back to login.
        setChallengeToken(null);
        setTotpCode("");
        setError(data?.error ?? t('twoFactor.failed'));
        return;
      }
      router.push(nextPath);
    } catch {
      setChallengeToken(null);
      setTotpCode("");
      setError(t('login.connectionError'));
    } finally {
      setLoading(false);
    }
  };

  const handleBack2FA = () => {
    setChallengeToken(null);
    setTotpCode("");
    setError(null);
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-[var(--bg)] relative overflow-hidden">
      {/* Ambient glow orbs */}
      <div
        className="absolute w-[400px] h-[400px] rounded-full pointer-events-none opacity-20 blur-[120px]"
        style={{ background: "var(--accent)", top: "-10%", right: "-5%" }}
      />
      <div
        className="absolute w-[300px] h-[300px] rounded-full pointer-events-none opacity-10 blur-[100px]"
        style={{ background: "var(--accent-secondary)", bottom: "-10%", left: "-5%" }}
      />

      {/* Dot grid */}
      <div
        className="absolute inset-0 pointer-events-none opacity-[0.03]"
        style={{
          backgroundImage: "radial-gradient(circle, var(--text) 1px, transparent 1px)",
          backgroundSize: "24px 24px",
        }}
      />

      {/* Login card with animated border */}
      <div className="relative w-full max-w-sm z-10">
        {/* Animated gradient border */}
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
          {/* Specular highlight */}
          <div
            className="absolute top-0 left-[10%] right-[10%] h-px pointer-events-none"
            style={{ background: `linear-gradient(90deg, transparent, color-mix(in srgb, var(--accent) 40%, white), transparent)` }}
          />

          {/* Logo + Heading */}
          <div className="flex flex-col items-center gap-3">
            <img src="/logo.svg" alt="LabTether" width={56} height={56} />
            <div className="text-center">
              <h1 className="text-xl font-medium text-[var(--text)] font-[family-name:var(--font-heading)]">{t('brand')}</h1>
              <p className="text-[10px] font-mono uppercase tracking-[0.15em] text-[var(--muted)] mt-0.5">{t('brandSub')}</p>
            </div>
          </div>

          {challengeToken ? (
            <>
              <p className="text-sm text-[var(--muted)] text-center">{t('twoFactor.subtitle')}</p>
              <form onSubmit={handleSubmit2FA} className="space-y-4">
                {error ? <div className="text-sm text-[var(--bad)] bg-[var(--bad-glow)] rounded-lg px-3 py-2">{error}</div> : null}
                <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)]">
                  {t('twoFactor.label')}
                  <Input
                    type="text"
                    inputMode="numeric"
                    pattern="[0-9a-zA-Z\-]*"
                    maxLength={20}
                    value={totpCode}
                    onChange={(e) => setTotpCode(e.target.value)}
                    autoFocus
                    autoComplete="one-time-code"
                    placeholder={t('twoFactor.placeholder')}
                    required
                  />
                </label>
                <Button type="submit" variant="primary" className="w-full" disabled={loading}>
                  {loading ? t('twoFactor.submitting') : t('twoFactor.submit')}
                </Button>
                <button
                  type="button"
                  className="w-full text-sm text-[var(--muted)] hover:text-[var(--text)] transition-colors"
                  onClick={handleBack2FA}
                >
                  {t('twoFactor.back')}
                </button>
              </form>
            </>
          ) : (
            <>
              <p className="text-sm text-[var(--muted)] text-center">{t('login.subtitle')}</p>
              {oidcEnabled ? (
                <Button type="button" variant="secondary" className="w-full" onClick={handleOIDCLogin}>
                  {oidcProviderName ? t('login.ssoProvider', { provider: oidcProviderName }) : t('login.ssoDefault')}
                </Button>
              ) : null}
              <form onSubmit={handleSubmit} className="space-y-4">
                {error ? <div className="text-sm text-[var(--bad)] bg-[var(--bad-glow)] rounded-lg px-3 py-2">{error}</div> : null}
                <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)]">
                  {t('login.username')}
                  <Input
                    type="text"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    autoComplete="username"
                    autoFocus
                    required
                  />
                </label>
                <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)]">
                  {t('login.password')}
                  <Input
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoComplete="current-password"
                    required
                  />
                </label>
                <Button type="submit" variant="primary" className="w-full" disabled={loading}>
                  {loading ? t('login.submitting') : t('login.submit')}
                </Button>
              </form>
            </>
          )}
        </Card>
      </div>
    </div>
  );
}

"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "../../../i18n/navigation";
import { useTranslations } from "next-intl";
import { Button } from "../../components/ui/Button";
import { safeLocalRedirectPath } from "../../../lib/safeRedirect";
import { SignInField, SignInShell } from "./SignInShell";

const LOCALE_PREFIXES = ["/en", "/de", "/fr", "/es", "/zh"];
// Codes the SSO routes put in ?error=. Anything else shows the generic one.
const SSO_ERROR_CODES = [
  "sso_unavailable",
  "sso_address_refused",
  "sso_expired",
  "sso_setup_required",
  "sso_failed",
] as const;

function safeNextPath(value: string | null): string {
  const safePath = safeLocalRedirectPath(value);
  // Strip locale prefix — next-intl's router.push adds it automatically.
  // Without this, /en → router.push("/en") → /en/en → 404.
  for (const prefix of LOCALE_PREFIXES) {
    if (safePath === prefix) return "/";
    if (safePath.startsWith(prefix + "/")) return safePath.slice(prefix.length);
  }
  return safePath;
}

/** Sign in: username and password, then a two-factor or recovery code for accounts that have it on. */
export default function LoginPage() {
  const router = useRouter();
  const t = useTranslations("auth");
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
  const [recovery, setRecovery] = useState(false);
  const [demoMode, setDemoMode] = useState(false);
  const submitAbortRef = useRef<AbortController | null>(null);
  const submitSeqRef = useRef(0);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    setNextPath(safeNextPath(params.get("next")));
    // Show only fixed messages. Text from the URL is never shown, so a crafted
    // link cannot put its own words on the real sign-in page.
    const errorParam = params.get("error");
    if (errorParam) {
      const code = (SSO_ERROR_CODES as readonly string[]).includes(errorParam) ? errorParam : "sso_failed";
      setError(t(`login.ssoErrors.${code}`, { address: window.location.origin }));
    }
  }, [t]);

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
    return () => {
      cancelled = true;
    };
  }, [nextPath, router]);

  useEffect(() => {
    return () => {
      submitAbortRef.current?.abort();
      submitAbortRef.current = null;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    async function checkDemoMode() {
      try {
        const response = await fetch("/api/demo/session", { method: "HEAD", cache: "no-store" });
        if (!cancelled && response.status === 200) {
          setDemoMode(true);
          setUsername("demo");
          setPassword("demo-viewer");
        }
      } catch {
        // Not in demo mode.
      }
    }
    void checkDemoMode();
    return () => {
      cancelled = true;
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
    return () => {
      cancelled = true;
    };
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
        setPassword("");
        setChallengeToken(data.challenge_token);
        setRecovery(false);
        setLoading(false);
        return;
      }

      if (!response.ok) {
        setPassword("");
        setError(data?.error ?? t("login.failed"));
        return;
      }

      router.push(nextPath);
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") {
        return;
      }
      setPassword("");
      setError(t("login.connectionError"));
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
        body: JSON.stringify({ challenge_token: challengeToken, code: totpCode.trim() }),
        credentials: "same-origin",
      });
      if (!response.ok) {
        const data = await response.json().catch(() => null);
        // The server consumes the challenge token on the first attempt. A retry
        // with the same token will always fail. Clear and send user back to login.
        setChallengeToken(null);
        setTotpCode("");
        setError(data?.error ?? t("twoFactor.failed"));
        return;
      }
      router.push(nextPath);
    } catch {
      setChallengeToken(null);
      setTotpCode("");
      setError(t("login.connectionError"));
    } finally {
      setLoading(false);
    }
  };

  const handleBack2FA = () => {
    setChallengeToken(null);
    setTotpCode("");
    setRecovery(false);
    setError(null);
  };

  if (challengeToken) {
    return (
      <SignInShell title={t("twoFactor.title")}>
        <p className="si-who">{username}</p>
        <form className="si-form" onSubmit={handleSubmit2FA} aria-label={t(recovery ? "twoFactor.recoveryLabel" : "twoFactor.label")}>
          {error ? <p className="si-err" role="alert">{error}</p> : null}
          <SignInField
            id="si-code"
            label={t(recovery ? "twoFactor.recoveryLabel" : "twoFactor.label")}
            className={recovery ? "si-rec" : "si-otp"}
            type="text"
            inputMode={recovery ? "text" : "numeric"}
            pattern={recovery ? "[0-9a-fA-F\\-]*" : "[0-9]*"}
            maxLength={recovery ? 20 : 6}
            value={totpCode}
            onChange={(e) => setTotpCode(e.target.value)}
            autoFocus
            autoComplete={recovery ? "off" : "one-time-code"}
            autoCapitalize="off"
            spellCheck={false}
            required
          />
          <Button type="submit" variant="primary" className="si-go" disabled={loading}>
            {loading ? t("twoFactor.submitting") : t("twoFactor.submit")}
          </Button>
        </form>
        <div className="si-alt">
          <Button variant="ghost" size="sm" onClick={() => { setRecovery(!recovery); setTotpCode(""); setError(null); }}>
            {recovery ? t("twoFactor.useCode") : t("twoFactor.useRecovery")}
          </Button>
          <Button variant="ghost" size="sm" onClick={handleBack2FA}>{t("twoFactor.back")}</Button>
        </div>
      </SignInShell>
    );
  }

  return (
    <SignInShell title={t("login.title")}>
      {demoMode ? <p className="si-note">{t("login.demo")}</p> : null}
      <form className="si-form" onSubmit={handleSubmit} aria-label={t("login.title")}>
        {error ? <p className="si-err" role="alert">{error}</p> : null}
        <SignInField
          id="si-user"
          label={t("login.username")}
          type="text"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoComplete="username"
          autoCapitalize="off"
          spellCheck={false}
          autoFocus
          required
        />
        <SignInField
          id="si-pass"
          label={t("login.password")}
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="current-password"
          required
        />
        <Button type="submit" variant="primary" className="si-go" disabled={loading}>
          {loading ? t("login.submitting") : t("login.submit")}
        </Button>
        {oidcEnabled ? (
          <Button type="button" variant="secondary" className="si-go si-sso" onClick={handleOIDCLogin}>
            {oidcProviderName ? t("login.ssoProvider", { provider: oidcProviderName }) : t("login.ssoDefault")}
          </Button>
        ) : null}
      </form>
    </SignInShell>
  );
}

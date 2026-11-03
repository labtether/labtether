"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "../../../i18n/navigation";
import { useTranslations } from "next-intl";
import { Button } from "../../components/ui/Button";
import { useTailscaleServeStatus } from "../../hooks/useTailscaleServeStatus";
import { runtimeSettingKeys } from "../../console/models";
import { safeLocalRedirectPath } from "../../../lib/safeRedirect";
import { buildLocalSetupDestination, buildRemoteAccessURL } from "./setupNavigation";
import { SetupRemoteAccessStep } from "./SetupRemoteAccessStep";
import { SignInField, SignInShell } from "../login/SignInShell";
import { copyText } from "../../lib/clipboard";

// How the compose install prints the one-time setup token (the installer prints the same line when it finishes).
const SETUP_TOKEN_COMMAND = "docker compose --env-file .env.deploy -f docker-compose.deploy.yml exec -T labtether sh -c 'cat /bootstrap/auth/setup-token'";

/** First-run setup: the one-time setup token and the owner account, then remote access. */
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
    void copyText(text).then(() => {
      setCopied(label);
      setTimeout(() => setCopied(""), 2000);
    }, () => setCopied(""));
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
        throw new Error(payload?.error || t("remoteAccess.managedError"));
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
      <main className="si">
        <p className="si-col si-wait" role="status">{t("setup.preparing")}</p>
      </main>
    );
  }

  if (step === "remote-access") {
    return (
      <SignInShell title={t("remoteAccess.title")} step={t("setup.step", { step: 2, total: 2 })} wide>
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
      </SignInShell>
    );
  }

  return (
    <SignInShell title={t("setup.title")} step={t("setup.step", { step: 1, total: 2 })} wide>
      <form onSubmit={handleSubmit} className="si-form" aria-label={t("setup.formLabel")}>
        {error ? <p className="si-err" role="alert">{error}</p> : null}
        <SignInField
          id="su-token"
          label={t("setup.setupToken")}
          className="si-mono"
          type="password"
          value={setupToken}
          onChange={(e) => setSetupToken(e.target.value)}
          autoFocus
          autoComplete="off"
          spellCheck={false}
          required
          maxLength={512}
          hint={(
            <>
              <span>{t("setup.tokenWhere")}</span>
              <span className="si-cmd">
                <code>{SETUP_TOKEN_COMMAND}</code>
                <Button type="button" variant="ghost" size="sm" onClick={() => copyToClipboard(SETUP_TOKEN_COMMAND, "token-command")}>
                  {copied === "token-command" ? t("copied") : t("copy")}
                </Button>
              </span>
              {/* The Hub also runs as a Home Assistant add-on, which keeps its token elsewhere (add-on DOCS.md). */}
              <span>{t("setup.tokenWhereAddon")}</span>
            </>
          )}
        />
        <SignInField
          id="su-user"
          label={t("setup.username")}
          type="text"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoComplete="username"
          autoCapitalize="off"
          spellCheck={false}
          required
        />
        <SignInField
          id="su-pass"
          label={t("setup.password")}
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="new-password"
          required
        />
        <SignInField
          id="su-pass2"
          label={t("setup.confirmPassword")}
          type="password"
          value={confirmPassword}
          onChange={(e) => setConfirmPassword(e.target.value)}
          autoComplete="new-password"
          error={passwordMismatch ? t("setup.passwordMismatch") : undefined}
          required
        />
        <Button
          type="submit"
          variant="primary"
         
          className="si-go"
          disabled={loading || passwordMismatch || setupToken.trim() === ""}
        >
          {loading ? t("setup.submitting") : t("setup.submit")}
        </Button>
      </form>
    </SignInShell>
  );
}

"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { useRouter } from "../../i18n/navigation";

type User = {
  id: string;
  username: string;
  role?: string;
  totp_enabled?: boolean;
};

type AuthContextValue = {
  user: User | null;
  loading: boolean;
  logout: () => Promise<void>;
  refreshUser: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [bootstrapError, setBootstrapError] = useState<string | null>(null);
  const [bootstrapAttempt, setBootstrapAttempt] = useState(0);

  const retryBootstrap = useCallback(() => {
    setBootstrapError(null);
    setLoading(true);
    setBootstrapAttempt((attempt) => attempt + 1);
  }, []);

  useEffect(() => {
    let cancelled = false;
    let retryTimer: number | null = null;
    const redirectToLogin = () => {
      const currentPath = window.location.pathname;
      const currentSearch = window.location.search;
      if (currentPath === "/login" || currentPath === "/setup") return;
      const nextPath = `${currentPath}${currentSearch}`;
      router.push(`/login?next=${encodeURIComponent(nextPath)}`);
    };

    const redirectToSetup = () => {
      const currentPath = window.location.pathname;
      const currentSearch = window.location.search;
      if (currentPath === "/setup") return;
      const nextPath = `${currentPath}${currentSearch}`;
      router.push(`/setup?next=${encodeURIComponent(nextPath)}`);
    };

    async function setupRequired() {
      try {
        const response = await fetch("/api/auth/bootstrap/status", { cache: "no-store" });
        if (!response.ok) {
          return false;
        }
        const payload = await response.json().catch(() => null);
        return Boolean(payload?.setup_required);
      } catch {
        return false;
      }
    }

    async function checkAuth() {
      const markTransientUnavailable = () => {
        setBootstrapError("LabTether can’t reach the backend right now. It may still be starting or restarting. We’ll keep retrying automatically.");
        if (!cancelled) {
          const backoffMs = Math.min(3000 * Math.pow(2, bootstrapAttempt), 60000);
          retryTimer = window.setTimeout(() => {
            if (!cancelled) {
              setBootstrapAttempt((attempt) => attempt + 1);
            }
          }, backoffMs);
        }
      };

      try {
        const response = await fetch("/api/auth/me", { cache: "no-store" });
        if (cancelled) {
          return;
        }

        if (response.ok) {
          const data = await response.json();
          setBootstrapError(null);
          setBootstrapAttempt(0);
          setUser(data.user ?? null);
          return;
        }

        if (response.status === 401 || response.status === 403) {
          setBootstrapError(null);
          setUser(null);
          if (await setupRequired()) {
            redirectToSetup();
          } else {
            redirectToLogin();
          }
          return;
        }

        markTransientUnavailable();
      } catch {
        if (!cancelled) {
          markTransientUnavailable();
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void checkAuth();
    return () => {
      cancelled = true;
      if (retryTimer) {
        window.clearTimeout(retryTimer);
      }
    };
  }, [bootstrapAttempt, router]);

  const refreshUser = useCallback(async () => {
    try {
      const response = await fetch("/api/auth/me", { cache: "no-store" });
      if (response.ok) {
        const data = await response.json();
        setUser(data.user ?? null);
      }
    } catch {
      // Ignore — keep current user state on refresh failure.
    }
  }, []);

  const logout = useCallback(async () => {
    try {
      await fetch("/api/auth/logout", { method: "POST" });
    } catch {
      // Ignore errors
    }
    setUser(null);
    router.push("/login");
  }, [router]);

  const value = useMemo<AuthContextValue>(
    () => ({ user, loading, logout, refreshUser }),
    [user, loading, logout, refreshUser]
  );

  if (bootstrapError && !user) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-[var(--bg)] px-6">
        <div className="max-w-md rounded-2xl border border-[var(--line)] bg-[var(--panel-glass)] p-6 text-center shadow-[var(--shadow-panel)]">
          <h1 className="text-base font-medium text-[var(--text)]">Session check unavailable</h1>
          <p className="mt-2 text-sm text-[var(--muted)]">{bootstrapError}</p>
          <p className="mt-2 text-xs text-[var(--muted)]">If you just restarted the backend, this page should recover on its own once the hub is reachable again.</p>
          <button
            type="button"
            onClick={retryBootstrap}
            className="mt-4 inline-flex items-center justify-center rounded-lg bg-[var(--accent)] px-4 py-2 text-sm font-medium text-black transition-opacity hover:opacity-90"
          >
            Try Again
          </button>
        </div>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-[var(--bg)]">
        <p className="text-sm text-[var(--muted)]">Loading...</p>
      </div>
    );
  }

  return (
    <AuthContext.Provider value={value}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}

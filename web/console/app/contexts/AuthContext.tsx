"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useRouter } from "../../i18n/navigation";
import { SessionChecking, SessionUnreachable } from "./SessionScreens";

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
  const [bootstrapError, setBootstrapError] = useState(false);
  const [bootstrapAttempt, setBootstrapAttempt] = useState(0);
  const sessionGeneration = useRef(0);
  const logoutStarted = useRef(false);

  const retryBootstrap = useCallback(() => {
    setBootstrapError(false);
    setLoading(true);
    setBootstrapAttempt((attempt) => attempt + 1);
  }, []);

  useEffect(() => {
    let cancelled = false;
    // The counter ref (not a DOM node) is bumped again on cleanup so any
    // in-flight bootstrap or refresh from this run is ignored.
    const generations = sessionGeneration;
    const generation = ++generations.current;
    const isCurrent = () => !cancelled && !logoutStarted.current
      && generation === generations.current;
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
        if (!isCurrent()) return;
        setBootstrapError(true);
        if (isCurrent()) {
          const backoffMs = Math.min(3000 * Math.pow(2, bootstrapAttempt), 60000);
          retryTimer = window.setTimeout(() => {
            if (isCurrent()) {
              setBootstrapAttempt((attempt) => attempt + 1);
            }
          }, backoffMs);
        }
      };

      try {
        const response = await fetch("/api/auth/me", { cache: "no-store" });
        if (!isCurrent()) {
          return;
        }

        if (response.ok) {
          const data = await response.json();
          if (!isCurrent()) return;
          setBootstrapError(false);
          setBootstrapAttempt(0);
          setUser(data.user ?? null);
          return;
        }

        if (response.status === 401 || response.status === 403) {
          setBootstrapError(false);
          setUser(null);
          const needsSetup = await setupRequired();
          if (!isCurrent()) return;
          if (needsSetup) {
            redirectToSetup();
          } else {
            redirectToLogin();
          }
          return;
        }

        markTransientUnavailable();
      } catch {
        if (isCurrent()) {
          markTransientUnavailable();
        }
      } finally {
        if (isCurrent()) {
          setLoading(false);
        }
      }
    }

    void checkAuth();
    return () => {
      cancelled = true;
      ++generations.current;
      if (retryTimer) {
        window.clearTimeout(retryTimer);
      }
    };
  }, [bootstrapAttempt, router]);

  const refreshUser = useCallback(async () => {
    if (logoutStarted.current) return;
    const generation = sessionGeneration.current;
    try {
      const response = await fetch("/api/auth/me", { cache: "no-store" });
      if (response.ok) {
        const data = await response.json();
        if (logoutStarted.current || generation !== sessionGeneration.current) return;
        setUser(data.user ?? null);
      }
    } catch {
      // Ignore — keep current user state on refresh failure.
    }
  }, []);

  const logout = useCallback(async () => {
    // Invalidate pending reads before waiting for the logout request.
    logoutStarted.current = true;
    ++sessionGeneration.current;
    setUser(null);
    setBootstrapError(false);
    setLoading(false);
    try {
      await fetch("/api/auth/logout", { method: "POST" });
    } catch {
      // Ignore errors
    }
    router.push("/login");
  }, [router]);

  const value = useMemo<AuthContextValue>(
    () => ({ user, loading, logout, refreshUser }),
    [user, loading, logout, refreshUser]
  );

  if (bootstrapError && !user) {
    return <SessionUnreachable onRetry={retryBootstrap} />;
  }

  if (loading) {
    return <SessionChecking />;
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

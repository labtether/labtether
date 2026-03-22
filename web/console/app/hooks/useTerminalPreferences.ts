"use client";

import { useState, useEffect, useCallback, useRef } from "react";

export interface TerminalPreferences {
  theme: string;
  font_family: string;
  font_size: number;
  cursor_style: "block" | "underline" | "bar";
  cursor_blink: boolean;
  scrollback: number;
  toolbar_keys: string[] | null;
  auto_reconnect: boolean;
}

const defaultPrefs: TerminalPreferences = {
  theme: "labtether-dark",
  font_family: "JetBrains Mono",
  font_size: 14,
  cursor_style: "block",
  cursor_blink: true,
  scrollback: 5000,
  toolbar_keys: null,
  auto_reconnect: false,
};

function unwrapPreferencesPayload(payload: unknown): Partial<TerminalPreferences> {
  if (typeof payload !== "object" || payload === null) {
    return {};
  }
  const record = payload as Record<string, unknown>;
  const nested = record.preferences;
  if (typeof nested === "object" && nested !== null) {
    return nested as Partial<TerminalPreferences>;
  }
  return record as Partial<TerminalPreferences>;
}

export function useTerminalPreferences() {
  const [prefs, setPrefs] = useState<TerminalPreferences>(defaultPrefs);
  const [loading, setLoading] = useState(true);
  const abortRef = useRef<AbortController | null>(null);
  const updateAbortRef = useRef<AbortController | null>(null);
  const mountedRef = useRef(true);

  const fetchPrefs = useCallback(async () => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    try {
      const res = await fetch("/api/terminal/preferences", {
        cache: "no-store",
        signal: controller.signal,
      });
      if (res.ok) {
        const data = unwrapPreferencesPayload(await res.json());
        if (mountedRef.current) {
          setPrefs({ ...defaultPrefs, ...data });
        }
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      // Keep defaults on error
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      if (mountedRef.current) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    void fetchPrefs();
    return () => {
      mountedRef.current = false;
      abortRef.current?.abort();
      abortRef.current = null;
      updateAbortRef.current?.abort();
      updateAbortRef.current = null;
    };
  }, [fetchPrefs]);

  const updatePrefs = useCallback(
    async (updates: Partial<TerminalPreferences>) => {
      // Optimistic update
      setPrefs((prev) => ({ ...prev, ...updates }));
      updateAbortRef.current?.abort();
      const controller = new AbortController();
      updateAbortRef.current = controller;
      try {
        const res = await fetch("/api/terminal/preferences", {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(updates),
          signal: controller.signal,
        });
        if (res.ok) {
          const data = unwrapPreferencesPayload(await res.json());
          if (mountedRef.current) {
            setPrefs({ ...defaultPrefs, ...data });
          }
        } else {
          await fetchPrefs();
        }
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError") {
          return;
        }
        // Revert on error by refetching
        await fetchPrefs();
      } finally {
        if (updateAbortRef.current === controller) {
          updateAbortRef.current = null;
        }
      }
    },
    [fetchPrefs]
  );

  return { prefs, updatePrefs, loading };
}

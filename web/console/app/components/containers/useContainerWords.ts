"use client";

import { useLocale, useTranslations } from "next-intl";
import { useCallback, useEffect, useState } from "react";
import type { Ago, CtState } from "./containerModel";

/** "2 days ago" in the page's language. */
export function useAgo() {
  const locale = useLocale();
  return useCallback((a: Ago) => {
    try {
      return new Intl.RelativeTimeFormat(locale, { numeric: "auto" }).format(-a.n, a.unit);
    } catch {
      return `${a.n} ${a.unit}`;
    }
  }, [locale]);
}

/** "6 days" in the page's language. */
export function useSpan() {
  const locale = useLocale();
  return useCallback((a: Ago) => {
    try {
      return new Intl.NumberFormat(locale, { style: "unit", unit: a.unit, unitDisplay: "long" }).format(a.n);
    } catch {
      return `${a.n} ${a.unit}`;
    }
  }, [locale]);
}

/**
 * The state word for a container: "" while plainly running, else "Exited 2 days ago", "Unhealthy", "Restarting", …
 * full: also say "Running" (a container's own page).
 */
export function useStateWord() {
  const t = useTranslations("containers.state");
  const ago = useAgo();
  return useCallback((st: CtState, full = false): string => {
    switch (st.kind) {
      case "running":
        if (st.health === "unhealthy") return t("unhealthy");
        if (st.health === "starting") return t("starting");
        return full ? t("running") : "";
      case "exited":
        return st.ago ? t("exitedAgo", { ago: ago(st.ago) }) : t("exited");
      case "restarting":
      case "paused":
      case "created":
      case "dead":
      case "removing":
        return t(st.kind);
      default:
        return t("unknown");
    }
  }, [t, ago]);
}

/** A set of ids kept in localStorage (closed sections). */
export function useStoredSet(key: string) {
  const [set, setSet] = useState<Set<string>>(() => new Set());
  useEffect(() => {
    try {
      const raw = window.localStorage.getItem(key);
      if (raw) setSet(new Set(JSON.parse(raw) as string[]));
    } catch { /* private window or a bad value: start empty */ }
  }, [key]);
  const flip = useCallback((id: string) => {
    setSet((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      try { window.localStorage.setItem(key, JSON.stringify([...next])); } catch { /* not kept */ }
      return next;
    });
  }, [key]);
  return [set, flip] as const;
}

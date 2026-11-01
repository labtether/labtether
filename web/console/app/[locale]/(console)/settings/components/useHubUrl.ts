"use client";

import { useEffect, useState } from "react";

/** The hub's address as agents and API clients reach it (the enrollment hub URL), else this page's origin. */
export function useHubUrl(): { hubUrl: string; loading: boolean } {
  const [hubUrl, setHubUrl] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    const finish = (url: string) => {
      if (cancelled) return;
      setHubUrl(url.replace(/\/+$/u, ""));
      setLoading(false);
    };
    fetch("/api/settings/enrollment", { cache: "no-store", signal: AbortSignal.timeout(10_000) })
      .then(async (res) => {
        if (!res.ok) throw new Error("enrollment unavailable");
        const data = (await res.json().catch(() => null)) as Record<string, unknown> | null;
        const url = typeof data?.hub_url === "string" ? data.hub_url.trim() : "";
        finish(url || window.location.origin);
      })
      .catch(() => finish(window.location.origin));
    return () => {
      cancelled = true;
    };
  }, []);

  return { hubUrl, loading };
}

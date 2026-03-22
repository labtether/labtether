"use client";

import { useEffect, useState } from "react";

export type TailscaleServeStatus = {
  tailscale_installed: boolean;
  backend_state?: string;
  logged_in: boolean;
  tailnet?: string;
  dns_name?: string;
  tailscale_ips?: string[];
  tsnet_url?: string;
  serve_status: string;
  serve_configured: boolean;
  serve_target?: string;
  suggested_target?: string;
  suggested_command?: string;
  recommendation_state: string;
  recommendation_message: string;
  can_manage: boolean;
  management_mode: string;
  desired_mode: string;
  desired_mode_source: "ui" | "docker" | "default";
  desired_target?: string;
  desired_target_source: "ui" | "docker" | "default";
  status_note?: string;
};

export function useTailscaleServeStatus(enabled = true) {
  const [status, setStatus] = useState<TailscaleServeStatus | null>(null);
  const [loading, setLoading] = useState(enabled);
  const [error, setError] = useState("");
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    if (!enabled) {
      setLoading(false);
      return;
    }

    let cancelled = false;

    async function load() {
      setLoading(true);
      try {
        const response = await fetch("/api/settings/tailscale/serve", { cache: "no-store" });
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`);
        }
        const payload = await response.json() as TailscaleServeStatus;
        if (cancelled) {
          return;
        }
        setStatus(payload);
        setError("");
      } catch (err: unknown) {
        if (cancelled) {
          return;
        }
        setError(err instanceof Error ? err.message : "Failed to load Tailscale HTTPS status");
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [enabled, reloadKey]);

  return {
    status,
    loading,
    error,
    refresh: () => setReloadKey((value) => value + 1),
  };
}

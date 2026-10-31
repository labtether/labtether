"use client";

import { useCallback, useEffect, useState } from "react";

export type HubSSHKeySetting = {
  enabled: boolean;
  removal_pending: boolean;
  agent_connected: boolean;
  removal_sent?: boolean;
};

export type HubSSHKeyError = "load" | "save" | "forbidden";

function parseSetting(payload: unknown): HubSSHKeySetting | null {
  if (!payload || typeof payload !== "object") return null;
  const record = payload as Record<string, unknown>;
  const enabled = record["enabled"];
  if (typeof enabled !== "boolean") return null;
  return {
    enabled,
    removal_pending: record["removal_pending"] === true,
    agent_connected: record["agent_connected"] === true,
    removal_sent: record["removal_sent"] === true,
  };
}

export function useHubSSHKeySetting(nodeId: string) {
  const [setting, setSetting] = useState<HubSSHKeySetting | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<HubSSHKeyError | null>(null);
  const [lastChange, setLastChange] = useState<"on" | "off" | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    setLastChange(null);
    void (async () => {
      try {
        const response = await fetch(`/api/assets/${encodeURIComponent(nodeId)}/hub-ssh-key`, { cache: "no-store" });
        const parsed = response.ok ? parseSetting(await response.json()) : null;
        if (cancelled) return;
        setSetting(parsed);
        if (!parsed) setError("load");
      } catch {
        if (!cancelled) setError("load");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [nodeId]);

  const setEnabled = useCallback(
    async (enabled: boolean) => {
      setSaving(true);
      setError(null);
      try {
        const response = await fetch(`/api/assets/${encodeURIComponent(nodeId)}/hub-ssh-key`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ enabled }),
        });
        if (response.status === 403) {
          setError("forbidden");
          return;
        }
        const parsed = response.ok ? parseSetting(await response.json()) : null;
        if (!parsed) {
          setError("save");
          return;
        }
        setSetting(parsed);
        setLastChange(parsed.enabled ? "on" : "off");
      } catch {
        setError("save");
      } finally {
        setSaving(false);
      }
    },
    [nodeId],
  );

  return { setting, loading, saving, error, lastChange, setEnabled };
}

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { sanitizeErrorMessage } from "../lib/sanitizeErrorMessage";

export type ProxmoxAuthMethod = "api_token" | "password";

type ProxmoxSettingsPayload = {
  configured?: boolean;
  collector_id?: string;
  credential_id?: string;
  credential_name?: string;
  settings?: {
    base_url?: string;
    auth_method?: ProxmoxAuthMethod;
    token_id?: string;
    username?: string;
    skip_verify?: boolean;
    ca_pem?: string;
    cluster_name?: string;
    interval_seconds?: number;
  };
  message?: string;
  warning?: string;
  result?: unknown;
  error?: string;
};

type ProxmoxSaveResult = {
  ok: boolean;
  collectorID?: string;
  warning?: string;
  error?: string;
};

function extractCollectorID(payload: ProxmoxSettingsPayload): string {
  const fromRoot = payload.collector_id?.trim();
  if (fromRoot) return fromRoot;

  const result = payload.result;
  if (!result || typeof result !== "object") {
    return "";
  }

  const resultRecord = result as Record<string, unknown>;
  const collector = resultRecord.collector;
  if (collector && typeof collector === "object") {
    const collectorID = (collector as Record<string, unknown>).id;
    if (typeof collectorID === "string" && collectorID.trim()) {
      return collectorID.trim();
    }
  }

  const resultCollectorID = resultRecord.collector_id;
  if (typeof resultCollectorID === "string" && resultCollectorID.trim()) {
    return resultCollectorID.trim();
  }

  return "";
}

export function useProxmoxSettings() {
  const [baseURL, setBaseURL] = useState("");
  const [authMethod, setAuthMethod] = useState<ProxmoxAuthMethod>("api_token");
  const [tokenID, setTokenID] = useState("");
  const [tokenSecret, setTokenSecret] = useState("");
  const [username, setUsername] = useState("");
  const [skipVerify, setSkipVerify] = useState(false);
  const [caPEM, setCAPEM] = useState("");
  const [clusterName, setClusterName] = useState("");
  const [intervalSeconds, setIntervalSeconds] = useState(60);
  const [collectorID, setCollectorID] = useState("");
  const [credentialID, setCredentialID] = useState("");
  const [credentialName, setCredentialName] = useState("");
  const [configured, setConfigured] = useState(false);

  const savingRef = useRef(false);

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    setMessage("");
    try {
      const response = await fetch("/api/settings/proxmox", { cache: "no-store", signal: AbortSignal.timeout(15_000) });
      const payload = (await response.json()) as ProxmoxSettingsPayload;
      if (!response.ok) {
        throw new Error(payload.error || `failed to load proxmox settings (${response.status})`);
      }

      const nextCollectorID = payload.collector_id?.trim() ?? "";
      setConfigured(Boolean(payload.configured));
      setCollectorID(nextCollectorID);
      setCredentialID(payload.credential_id ?? "");
      setCredentialName(payload.credential_name ?? "");
      setBaseURL(payload.settings?.base_url ?? "");
      setAuthMethod(payload.settings?.auth_method ?? "api_token");
      setTokenID(payload.settings?.token_id ?? "");
      setUsername(payload.settings?.username ?? "");
      setSkipVerify(payload.settings?.skip_verify ?? false);
      setCAPEM(payload.settings?.ca_pem ?? "");
      setClusterName(payload.settings?.cluster_name ?? "");
      setIntervalSeconds(payload.settings?.interval_seconds ?? 60);
      return nextCollectorID;
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "failed to load proxmox settings"));
      return "";
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const save = useCallback(async (): Promise<ProxmoxSaveResult> => {
    if (savingRef.current) {
      return { ok: false, error: "save already in progress" };
    }
    savingRef.current = true;
    setSaving(true);
    setError("");
    setMessage("");
    try {
      const body: Record<string, unknown> = {
        base_url: baseURL,
        auth_method: authMethod,
        skip_verify: skipVerify,
        ca_pem: caPEM,
        cluster_name: clusterName,
        interval_seconds: intervalSeconds,
      };
      if (authMethod === "password") {
        body.username = username;
        // Reuse tokenSecret field for the password value
        body.token_secret = tokenSecret;
      } else {
        body.token_id = tokenID;
        body.token_secret = tokenSecret;
      }

      const response = await fetch("/api/settings/proxmox", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        signal: AbortSignal.timeout(30_000),
        body: JSON.stringify(body)
      });
      const payload = (await response.json()) as ProxmoxSettingsPayload;
      if (!response.ok) {
        throw new Error(payload.error || `failed to save proxmox settings (${response.status})`);
      }

      const savedCollectorID = extractCollectorID(payload);
      setTokenSecret("");
      const loadedCollectorID = await load();
      setMessage(payload.warning ? `Proxmox settings saved. ${payload.warning}` : "Proxmox settings saved.");
      return {
        ok: true,
        collectorID: savedCollectorID || loadedCollectorID,
        warning: payload.warning,
      };
    } catch (err) {
      const errorMessage = sanitizeErrorMessage(
        err instanceof Error ? err.message : "",
        "failed to save proxmox settings",
        [tokenSecret],
      );
      setError(errorMessage);
      return { ok: false, error: errorMessage };
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  }, [baseURL, authMethod, tokenID, tokenSecret, username, skipVerify, caPEM, clusterName, intervalSeconds, load]);

  const testConnection = useCallback(async () => {
    setTesting(true);
    setError("");
    setMessage("");
    try {
      const body: Record<string, unknown> = {
        base_url: baseURL,
        auth_method: authMethod,
        credential_id: credentialID,
        skip_verify: skipVerify,
        ca_pem: caPEM,
      };
      if (authMethod === "password") {
        body.username = username;
        body.password = tokenSecret;
      } else {
        body.token_id = tokenID;
        body.token_secret = tokenSecret;
      }

      const response = await fetch("/api/settings/proxmox/test", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        signal: AbortSignal.timeout(25_000),
        body: JSON.stringify(body)
      });
      const payload = (await response.json()) as ProxmoxSettingsPayload;
      if (!response.ok) {
        throw new Error(payload.error || `connection test failed (${response.status})`);
      }
      setMessage(payload.message || "Proxmox connection succeeded.");
    } catch (err) {
      setError(
        sanitizeErrorMessage(
          err instanceof Error ? err.message : "",
          "failed to test proxmox connection",
          [tokenSecret],
        ),
      );
    } finally {
      setTesting(false);
    }
  }, [baseURL, authMethod, tokenID, tokenSecret, username, credentialID, skipVerify, caPEM]);

  const runNow = useCallback(async () => {
    if (!collectorID) {
      setError("Save the collector first, then run it.");
      return;
    }
    setRunning(true);
    setError("");
    setMessage("");
    try {
      const response = await fetch(
        `/api/settings/collectors/${encodeURIComponent(collectorID)}/run`,
        {
          method: "POST",
          signal: AbortSignal.timeout(20_000),
        },
      );
      const payload = (await response.json()) as ProxmoxSettingsPayload;
      if (!response.ok) {
        throw new Error(payload.error || `failed to run proxmox collector (${response.status})`);
      }
      setMessage(payload.message || "Collector run started.");
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "failed to run proxmox collector"));
    } finally {
      setRunning(false);
    }
  }, [collectorID]);

  return {
    baseURL,
    setBaseURL,
    authMethod,
    setAuthMethod,
    tokenID,
    setTokenID,
    tokenSecret,
    setTokenSecret,
    username,
    setUsername,
    skipVerify,
    setSkipVerify,
    caPEM,
    setCAPEM,
    clusterName,
    setClusterName,
    intervalSeconds,
    setIntervalSeconds,
    collectorID,
    credentialID,
    credentialName,
    configured,
    loading,
    saving,
    testing,
    running,
    error,
    message,
    load,
    save,
    testConnection,
    runNow,
  };
}

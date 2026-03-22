"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { sanitizeErrorMessage } from "../lib/sanitizeErrorMessage";
import type { ProxmoxAuthMethod } from "./useProxmoxSettings";

type CollectorSettingsPayload = {
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
  error?: string;
};

export function useCollectorSettings(collectorId: string | null) {
  const [baseURL, setBaseURL] = useState("");
  const [authMethod, setAuthMethod] = useState<ProxmoxAuthMethod>("api_token");
  const [tokenID, setTokenID] = useState("");
  const [tokenSecret, setTokenSecret] = useState("");
  const [username, setUsername] = useState("");
  const [skipVerify, setSkipVerify] = useState(false);
  const [caPEM, setCAPEM] = useState("");
  const [clusterName, setClusterName] = useState("");
  const [intervalSeconds, setIntervalSeconds] = useState(60);
  const [credentialID, setCredentialID] = useState("");
  const [credentialName, setCredentialName] = useState("");
  const [configured, setConfigured] = useState(false);

  const savingRef = useRef(false);

  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const load = useCallback(async () => {
    if (!collectorId) return;
    setLoading(true);
    setError("");
    setMessage("");
    try {
      const response = await fetch(
        `/api/settings/proxmox/${encodeURIComponent(collectorId)}`,
        { cache: "no-store", signal: AbortSignal.timeout(15_000) },
      );
      const payload = (await response.json()) as CollectorSettingsPayload;
      if (!response.ok) {
        throw new Error(payload.error || `failed to load collector settings (${response.status})`);
      }

      setConfigured(Boolean(payload.configured));
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
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "failed to load collector settings"));
    } finally {
      setLoading(false);
    }
  }, [collectorId]);

  useEffect(() => {
    if (collectorId) {
      void load();
    }
  }, [collectorId, load]);

  const save = useCallback(async () => {
    if (!collectorId || savingRef.current) return;
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
        body.token_secret = tokenSecret;
      } else {
        body.token_id = tokenID;
        body.token_secret = tokenSecret;
      }

      const response = await fetch(
        `/api/settings/proxmox/${encodeURIComponent(collectorId)}`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          signal: AbortSignal.timeout(30_000),
          body: JSON.stringify(body),
        },
      );
      const payload = (await response.json()) as CollectorSettingsPayload;
      if (!response.ok) {
        throw new Error(payload.error || `failed to save collector settings (${response.status})`);
      }
      setMessage("Collector settings saved.");
      setTokenSecret("");
      await load();
    } catch (err) {
      setError(
        sanitizeErrorMessage(
          err instanceof Error ? err.message : "",
          "failed to save collector settings",
          [tokenSecret],
        ),
      );
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  }, [collectorId, baseURL, authMethod, tokenID, tokenSecret, username, skipVerify, caPEM, clusterName, intervalSeconds, load]);

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
        body: JSON.stringify(body),
      });
      const payload = (await response.json()) as CollectorSettingsPayload;
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
    if (!collectorId) return;
    setRunning(true);
    setError("");
    setMessage("");
    try {
      const response = await fetch(
        `/api/settings/collectors/${encodeURIComponent(collectorId)}/run`,
        {
          method: "POST",
          signal: AbortSignal.timeout(20_000),
        },
      );
      const payload = (await response.json()) as { message?: string; error?: string };
      if (!response.ok) {
        throw new Error(payload.error || `failed to run collector (${response.status})`);
      }
      setMessage(payload.message || "Collector run started.");
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "failed to run collector"));
    } finally {
      setRunning(false);
    }
  }, [collectorId]);

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

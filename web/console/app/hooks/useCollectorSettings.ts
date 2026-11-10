"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { readHubReply, useConnectorWords } from "./connectorWords";
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
  error?: string;
  release?: string;
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
  const words = useConnectorWords("Proxmox");

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
      const payload = await readHubReply<CollectorSettingsPayload>(response);

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
      setError(words.loadFailed(err));
    } finally {
      setLoading(false);
    }
  }, [collectorId, words]);

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
      await readHubReply<CollectorSettingsPayload>(response);
      setTokenSecret("");
      await load();
      setMessage(words.saved());
    } catch (err) {
      setError(words.saveFailed(err, [tokenSecret]));
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  }, [collectorId, baseURL, authMethod, tokenID, tokenSecret, username, skipVerify, caPEM, clusterName, intervalSeconds, load, words]);

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
      setMessage(words.testOk(await readHubReply<CollectorSettingsPayload>(response)));
    } catch (err) {
      setError(words.testFailed(err, [tokenSecret]));
    } finally {
      setTesting(false);
    }
  }, [baseURL, authMethod, tokenID, tokenSecret, username, credentialID, skipVerify, caPEM, words]);

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
      await readHubReply<{ error?: string }>(response);
      setMessage(words.syncStarted());
    } catch (err) {
      setError(words.syncFailed(err));
    } finally {
      setRunning(false);
    }
  }, [collectorId, words]);

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

"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { readHubReply, useConnectorWords } from "./connectorWords";

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
    spice_skip_verify?: boolean;
    ca_pem?: string;
    cluster_name?: string;
    interval_seconds?: number;
  };
  warning?: string;
  result?: unknown;
  error?: string;
  release?: string;
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
  const [spiceSkipVerify, setSPICESkipVerify] = useState(false);
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
  const words = useConnectorWords("Proxmox");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    setMessage("");
    try {
      const response = await fetch("/api/settings/proxmox", { cache: "no-store", signal: AbortSignal.timeout(15_000) });
      const payload = await readHubReply<ProxmoxSettingsPayload>(response);

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
      setSPICESkipVerify(payload.settings?.spice_skip_verify ?? false);
      setCAPEM(payload.settings?.ca_pem ?? "");
      setClusterName(payload.settings?.cluster_name ?? "");
      setIntervalSeconds(payload.settings?.interval_seconds ?? 60);
      return nextCollectorID;
    } catch (err) {
      setError(words.loadFailed(err));
      return "";
    } finally {
      setLoading(false);
    }
  }, [words]);

  useEffect(() => {
    void load();
  }, [load]);

  const save = useCallback(async (): Promise<ProxmoxSaveResult> => {
    if (savingRef.current) {
      return { ok: false, error: words.saveBusy() };
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
        spice_skip_verify: spiceSkipVerify,
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
      const payload = await readHubReply<ProxmoxSettingsPayload>(response);

      const savedCollectorID = extractCollectorID(payload);
      setTokenSecret("");
      const loadedCollectorID = await load();
      setMessage(words.saved(payload.warning));
      return {
        ok: true,
        collectorID: savedCollectorID || loadedCollectorID,
        warning: payload.warning,
      };
    } catch (err) {
      const errorMessage = words.saveFailed(err, [tokenSecret]);
      setError(errorMessage);
      return { ok: false, error: errorMessage };
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  }, [baseURL, authMethod, tokenID, tokenSecret, username, skipVerify, spiceSkipVerify, caPEM, clusterName, intervalSeconds, load, words]);

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
      setMessage(words.testOk(await readHubReply<ProxmoxSettingsPayload>(response)));
    } catch (err) {
      setError(words.testFailed(err, [tokenSecret]));
    } finally {
      setTesting(false);
    }
  }, [baseURL, authMethod, tokenID, tokenSecret, username, credentialID, skipVerify, caPEM, words]);

  const runNow = useCallback(async () => {
    if (!collectorID) {
      setError(words.saveFirst());
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
      await readHubReply<ProxmoxSettingsPayload>(response);
      setMessage(words.syncStarted());
    } catch (err) {
      setError(words.syncFailed(err));
    } finally {
      setRunning(false);
    }
  }, [collectorID, words]);

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
    spiceSkipVerify,
    setSPICESkipVerify,
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

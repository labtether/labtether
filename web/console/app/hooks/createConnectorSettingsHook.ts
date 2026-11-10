"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { readHubReply, useConnectorWords } from "./connectorWords";

type ConnectorSettingsPayload = {
  configured?: boolean;
  collector_id?: string;
  credential_id?: string;
  credential_name?: string;
  warning?: string;
  result?: unknown;
  error?: string;
  release?: string;
  visible_datastores?: number;
};

type AdapterState<
  TPayload extends ConnectorSettingsPayload,
  TFields extends Record<string, unknown>,
> = {
  fields: TFields;
  applyLoadedSettings: (payload: TPayload) => void;
  buildSaveBody: () => Record<string, unknown>;
  buildTestBody: (credentialID: string) => Record<string, unknown>;
  clearSensitiveFields: () => void;
  getSensitiveValues: () => string[];
};

type ConnectorSettingsHookAdapter<
  TPayload extends ConnectorSettingsPayload,
  TFields extends Record<string, unknown>,
> = {
  settingsPath: string;
  displayName: string;
  useAdapterState: () => AdapterState<TPayload, TFields>;
  loadRequestInit?: RequestInit;
  saveRequestInit?: RequestInit;
  testRequestInit?: RequestInit;
  runRequestInit?: RequestInit;
};

export type ConnectorSaveResult = {
  ok: boolean;
  collectorID?: string;
  warning?: string;
  error?: string;
};

type ConnectorSettingsHookReturn<TFields extends Record<string, unknown>> = TFields & {
  collectorID: string;
  credentialID: string;
  credentialName: string;
  configured: boolean;
  loading: boolean;
  saving: boolean;
  testing: boolean;
  running: boolean;
  error: string;
  message: string;
  load: () => Promise<string>;
  save: () => Promise<ConnectorSaveResult>;
  testConnection: () => Promise<void>;
  runNow: () => Promise<void>;
};

function extractCollectorID(payload: ConnectorSettingsPayload): string {
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

export function createConnectorSettingsHook<
  TPayload extends ConnectorSettingsPayload,
  TFields extends Record<string, unknown>,
>(
  adapter: ConnectorSettingsHookAdapter<TPayload, TFields>,
): () => ConnectorSettingsHookReturn<TFields> {
  return function useConnectorSettings() {
    const {
      fields,
      applyLoadedSettings,
      buildSaveBody,
      buildTestBody,
      clearSensitiveFields,
      getSensitiveValues,
    } = adapter.useAdapterState();
    const words = useConnectorWords(adapter.displayName);

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
        const response = await fetch(adapter.settingsPath, {
          cache: "no-store",
          ...adapter.loadRequestInit,
        });
        const payload = await readHubReply<TPayload>(response);

        const nextCollectorID = payload.collector_id?.trim() ?? "";
        setConfigured(Boolean(payload.configured));
        setCollectorID(nextCollectorID);
        setCredentialID(payload.credential_id ?? "");
        setCredentialName(payload.credential_name ?? "");
        applyLoadedSettings(payload);
        return nextCollectorID;
      } catch (err) {
        setError(words.loadFailed(err));
        return "";
      } finally {
        setLoading(false);
      }
    }, [applyLoadedSettings, words]);

    useEffect(() => {
      void load();
    }, [load]);

    const save = useCallback(async (): Promise<ConnectorSaveResult> => {
      if (savingRef.current) {
        return { ok: false, error: words.saveBusy() };
      }
      savingRef.current = true;
      setSaving(true);
      setError("");
      setMessage("");
      try {
        const response = await fetch(adapter.settingsPath, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(buildSaveBody()),
          ...adapter.saveRequestInit,
        });
        const payload = await readHubReply<TPayload>(response);

        const savedCollectorID = extractCollectorID(payload);
        clearSensitiveFields();
        const loadedCollectorID = await load();
        setMessage(words.saved(payload.warning));
        return {
          ok: true,
          collectorID: savedCollectorID || loadedCollectorID,
          warning: payload.warning,
        };
      } catch (err) {
        const errorMessage = words.saveFailed(err, getSensitiveValues());
        setError(errorMessage);
        return { ok: false, error: errorMessage };
      } finally {
        savingRef.current = false;
        setSaving(false);
      }
    }, [buildSaveBody, clearSensitiveFields, getSensitiveValues, load, words]);

    const testConnection = useCallback(async () => {
      setTesting(true);
      setError("");
      setMessage("");
      try {
        const response = await fetch(`${adapter.settingsPath}/test`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(buildTestBody(credentialID)),
          ...adapter.testRequestInit,
        });
        setMessage(words.testOk(await readHubReply<TPayload>(response)));
      } catch (err) {
        setError(words.testFailed(err, getSensitiveValues()));
      } finally {
        setTesting(false);
      }
    }, [buildTestBody, credentialID, getSensitiveValues, words]);

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
            ...adapter.runRequestInit,
          },
        );
        await readHubReply<TPayload>(response);
        setMessage(words.syncStarted());
      } catch (err) {
        setError(words.syncFailed(err));
      } finally {
        setRunning(false);
      }
    }, [collectorID, words]);

    return {
      ...fields,
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
  };
}

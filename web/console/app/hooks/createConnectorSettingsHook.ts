"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { sanitizeErrorMessage } from "../lib/sanitizeErrorMessage";

type ConnectorSettingsPayload = {
  configured?: boolean;
  collector_id?: string;
  credential_id?: string;
  credential_name?: string;
  message?: string;
  warning?: string;
  result?: unknown;
  error?: string;
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
  messageKey: string;
  testSuccessMessage: string;
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
        const payload = (await response.json()) as TPayload;
        if (!response.ok) {
          throw new Error(payload.error || `failed to load ${adapter.messageKey} settings (${response.status})`);
        }

        const nextCollectorID = payload.collector_id?.trim() ?? "";
        setConfigured(Boolean(payload.configured));
        setCollectorID(nextCollectorID);
        setCredentialID(payload.credential_id ?? "");
        setCredentialName(payload.credential_name ?? "");
        applyLoadedSettings(payload);
        return nextCollectorID;
      } catch (err) {
        setError(
          sanitizeErrorMessage(
            err instanceof Error ? err.message : "",
            `failed to load ${adapter.messageKey} settings`,
          ),
        );
        return "";
      } finally {
        setLoading(false);
      }
    }, [applyLoadedSettings]);

    useEffect(() => {
      void load();
    }, [load]);

    const save = useCallback(async (): Promise<ConnectorSaveResult> => {
      if (savingRef.current) {
        return { ok: false, error: "save already in progress" };
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
        const payload = (await response.json()) as TPayload;
        if (!response.ok) {
          throw new Error(payload.error || `failed to save ${adapter.messageKey} settings (${response.status})`);
        }

        const savedCollectorID = extractCollectorID(payload);
        clearSensitiveFields();
        const loadedCollectorID = await load();
        const saveSuccessMessage = `${adapter.displayName} settings saved.`;
        setMessage(payload.warning ? `${saveSuccessMessage} ${payload.warning}` : saveSuccessMessage);
        return {
          ok: true,
          collectorID: savedCollectorID || loadedCollectorID,
          warning: payload.warning,
        };
      } catch (err) {
        const errorMessage = sanitizeErrorMessage(
          err instanceof Error ? err.message : "",
          `failed to save ${adapter.messageKey} settings`,
          getSensitiveValues(),
        );
        setError(errorMessage);
        return { ok: false, error: errorMessage };
      } finally {
        savingRef.current = false;
        setSaving(false);
      }
    }, [buildSaveBody, clearSensitiveFields, getSensitiveValues, load]);

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
        const payload = (await response.json()) as TPayload;
        if (!response.ok) {
          throw new Error(payload.error || `connection test failed (${response.status})`);
        }
        setMessage(payload.message || adapter.testSuccessMessage);
      } catch (err) {
        setError(
          sanitizeErrorMessage(
            err instanceof Error ? err.message : "",
            `failed to test ${adapter.messageKey} connection`,
            getSensitiveValues(),
          ),
        );
      } finally {
        setTesting(false);
      }
    }, [buildTestBody, credentialID, getSensitiveValues]);

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
            ...adapter.runRequestInit,
          },
        );
        const payload = (await response.json()) as TPayload;
        if (!response.ok) {
          throw new Error(payload.error || `failed to run ${adapter.messageKey} collector (${response.status})`);
        }
        setMessage(payload.message || "Collector run started.");
      } catch (err) {
        setError(
          sanitizeErrorMessage(
            err instanceof Error ? err.message : "",
            `failed to run ${adapter.messageKey} collector`,
          ),
        );
      } finally {
        setRunning(false);
      }
    }, [collectorID]);

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

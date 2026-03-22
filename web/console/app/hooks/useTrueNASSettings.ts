"use client";

import { useCallback, useState } from "react";

import { createConnectorSettingsHook } from "./createConnectorSettingsHook";

type TrueNASSettingsPayload = {
  configured?: boolean;
  collector_id?: string;
  credential_id?: string;
  credential_name?: string;
  settings?: {
    base_url?: string;
    display_name?: string;
    skip_verify?: boolean;
    interval_seconds?: number;
  };
  message?: string;
  warning?: string;
  result?: unknown;
  error?: string;
};

const useTrueNASConnectorSettings = createConnectorSettingsHook({
  settingsPath: "/api/settings/truenas",
  displayName: "TrueNAS",
  messageKey: "truenas",
  testSuccessMessage: "TrueNAS connection succeeded.",
  useAdapterState: () => {
    const [baseURL, setBaseURL] = useState("");
    const [apiKey, setApiKey] = useState("");
    const [displayName, setDisplayName] = useState("");
    const [skipVerify, setSkipVerify] = useState(false);
    const [intervalSeconds, setIntervalSeconds] = useState(60);

    const applyLoadedSettings = useCallback((payload: TrueNASSettingsPayload) => {
      setBaseURL(payload.settings?.base_url ?? "");
      setDisplayName(payload.settings?.display_name ?? "");
      setSkipVerify(payload.settings?.skip_verify ?? false);
      setIntervalSeconds(payload.settings?.interval_seconds ?? 60);
    }, []);

    const buildSaveBody = useCallback(
      () => ({
        base_url: baseURL,
        api_key: apiKey,
        display_name: displayName,
        skip_verify: skipVerify,
        interval_seconds: intervalSeconds,
      }),
      [baseURL, apiKey, displayName, skipVerify, intervalSeconds],
    );

    const buildTestBody = useCallback(
      (credentialID: string) => ({
        base_url: baseURL,
        api_key: apiKey,
        credential_id: credentialID,
        skip_verify: skipVerify,
      }),
      [baseURL, apiKey, skipVerify],
    );

    const clearSensitiveFields = useCallback(() => {
      setApiKey("");
    }, []);

    const getSensitiveValues = useCallback(() => [apiKey], [apiKey]);

    return {
      fields: {
        baseURL,
        setBaseURL,
        apiKey,
        setApiKey,
        displayName,
        setDisplayName,
        skipVerify,
        setSkipVerify,
        intervalSeconds,
        setIntervalSeconds,
      },
      applyLoadedSettings,
      buildSaveBody,
      buildTestBody,
      clearSensitiveFields,
      getSensitiveValues,
    };
  },
});

export function useTrueNASSettings() {
  return useTrueNASConnectorSettings();
}

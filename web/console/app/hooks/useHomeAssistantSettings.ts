"use client";

import { useCallback, useState } from "react";

import { createConnectorSettingsHook } from "./createConnectorSettingsHook";

type HomeAssistantSettingsPayload = {
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

const useHomeAssistantConnectorSettings = createConnectorSettingsHook({
  settingsPath: "/api/settings/homeassistant",
  displayName: "Home Assistant",
  messageKey: "home assistant",
  testSuccessMessage: "Home Assistant connection succeeded.",
  useAdapterState: () => {
    const [baseURL, setBaseURL] = useState("");
    const [token, setToken] = useState("");
    const [displayName, setDisplayName] = useState("");
    const [skipVerify, setSkipVerify] = useState(false);
    const [intervalSeconds, setIntervalSeconds] = useState(60);

    const applyLoadedSettings = useCallback((payload: HomeAssistantSettingsPayload) => {
      setBaseURL(payload.settings?.base_url ?? "");
      setDisplayName(payload.settings?.display_name ?? "");
      setSkipVerify(payload.settings?.skip_verify ?? false);
      setIntervalSeconds(payload.settings?.interval_seconds ?? 60);
    }, []);

    const buildSaveBody = useCallback(
      () => ({
        base_url: baseURL,
        token,
        display_name: displayName,
        skip_verify: skipVerify,
        interval_seconds: intervalSeconds,
      }),
      [baseURL, token, displayName, skipVerify, intervalSeconds],
    );

    const buildTestBody = useCallback(
      (credentialID: string) => ({
        base_url: baseURL,
        token,
        credential_id: credentialID,
        skip_verify: skipVerify,
      }),
      [baseURL, token, skipVerify],
    );

    const clearSensitiveFields = useCallback(() => {
      setToken("");
    }, []);

    const getSensitiveValues = useCallback(() => [token], [token]);

    return {
      fields: {
        baseURL,
        setBaseURL,
        token,
        setToken,
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

export function useHomeAssistantSettings() {
  return useHomeAssistantConnectorSettings();
}

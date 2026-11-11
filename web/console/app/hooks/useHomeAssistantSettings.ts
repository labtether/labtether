"use client";

import { useCallback, useState } from "react";

import { isLocalNetworkHTTPAddress } from "../../lib/localNetworkAddress";
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
    allow_local_http?: boolean;
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
  useAdapterState: () => {
    const [baseURL, setBaseURL] = useState("");
    const [token, setToken] = useState("");
    const [displayName, setDisplayName] = useState("");
    const [skipVerify, setSkipVerify] = useState(false);
    const [allowLocalHTTP, setAllowLocalHTTP] = useState(false);
    const [intervalSeconds, setIntervalSeconds] = useState(60);
    // Sent only while the address is one the switch is offered for.
    const localHTTP = allowLocalHTTP && isLocalNetworkHTTPAddress(baseURL);

    const applyLoadedSettings = useCallback((payload: HomeAssistantSettingsPayload) => {
      setBaseURL(payload.settings?.base_url ?? "");
      setDisplayName(payload.settings?.display_name ?? "");
      setSkipVerify(payload.settings?.skip_verify ?? false);
      setAllowLocalHTTP(payload.settings?.allow_local_http ?? false);
      setIntervalSeconds(payload.settings?.interval_seconds ?? 60);
    }, []);

    const buildSaveBody = useCallback(
      () => ({
        base_url: baseURL,
        token,
        display_name: displayName,
        skip_verify: skipVerify,
        allow_local_http: localHTTP,
        interval_seconds: intervalSeconds,
      }),
      [baseURL, token, displayName, skipVerify, localHTTP, intervalSeconds],
    );

    const buildTestBody = useCallback(
      (credentialID: string) => ({
        base_url: baseURL,
        token,
        credential_id: credentialID,
        skip_verify: skipVerify,
        allow_local_http: localHTTP,
      }),
      [baseURL, token, skipVerify, localHTTP],
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
        allowLocalHTTP,
        setAllowLocalHTTP,
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

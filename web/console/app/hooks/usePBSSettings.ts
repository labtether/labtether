"use client";

import { useCallback, useState } from "react";

import { createConnectorSettingsHook } from "./createConnectorSettingsHook";

type PBSSettingsPayload = {
  configured?: boolean;
  collector_id?: string;
  credential_id?: string;
  credential_name?: string;
  settings?: {
    base_url?: string;
    token_id?: string;
    display_name?: string;
    skip_verify?: boolean;
    ca_pem?: string;
    interval_seconds?: number;
  };
  message?: string;
  warning?: string;
  result?: unknown;
  error?: string;
};

const usePBSConnectorSettings = createConnectorSettingsHook({
  settingsPath: "/api/settings/pbs",
  displayName: "PBS",
  messageKey: "pbs",
  testSuccessMessage: "PBS connection succeeded.",
  useAdapterState: () => {
    const [baseURL, setBaseURL] = useState("");
    const [tokenID, setTokenID] = useState("");
    const [tokenSecret, setTokenSecret] = useState("");
    const [displayName, setDisplayName] = useState("");
    const [skipVerify, setSkipVerify] = useState(false);
    const [caPEM, setCAPEM] = useState("");
    const [intervalSeconds, setIntervalSeconds] = useState(60);

    const applyLoadedSettings = useCallback((payload: PBSSettingsPayload) => {
      setBaseURL(payload.settings?.base_url ?? "");
      setTokenID(payload.settings?.token_id ?? "");
      setDisplayName(payload.settings?.display_name ?? "");
      setSkipVerify(payload.settings?.skip_verify ?? false);
      setCAPEM(payload.settings?.ca_pem ?? "");
      setIntervalSeconds(payload.settings?.interval_seconds ?? 60);
    }, []);

    const buildSaveBody = useCallback(
      () => ({
        base_url: baseURL,
        token_id: tokenID,
        token_secret: tokenSecret,
        display_name: displayName,
        skip_verify: skipVerify,
        ca_pem: caPEM,
        interval_seconds: intervalSeconds,
      }),
      [baseURL, tokenID, tokenSecret, displayName, skipVerify, caPEM, intervalSeconds],
    );

    const buildTestBody = useCallback(
      (credentialID: string) => ({
        base_url: baseURL,
        token_id: tokenID,
        token_secret: tokenSecret,
        credential_id: credentialID,
        skip_verify: skipVerify,
        ca_pem: caPEM,
      }),
      [baseURL, tokenID, tokenSecret, skipVerify, caPEM],
    );

    const clearSensitiveFields = useCallback(() => {
      setTokenSecret("");
    }, []);

    const getSensitiveValues = useCallback(() => [tokenSecret], [tokenSecret]);

    return {
      fields: {
        baseURL,
        setBaseURL,
        tokenID,
        setTokenID,
        tokenSecret,
        setTokenSecret,
        displayName,
        setDisplayName,
        skipVerify,
        setSkipVerify,
        caPEM,
        setCAPEM,
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

export function usePBSSettings() {
  return usePBSConnectorSettings();
}

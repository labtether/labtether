"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import { readError, readJSON, readUpdatedAssetName } from "./agentSettingsModel";

type UseAgentDeviceNameArgs = {
  nodeId: string;
  assetName: string;
  fetchStatus: () => Promise<void>;
};

export function useAgentDeviceName({ nodeId, assetName, fetchStatus }: UseAgentDeviceNameArgs) {
  const [deviceNameDraft, setDeviceNameDraft] = useState(assetName);
  const [savingDeviceName, setSavingDeviceName] = useState(false);
  const [deviceNameError, setDeviceNameError] = useState<string | null>(null);
  const [deviceNameMessage, setDeviceNameMessage] = useState<string | null>(null);

  useEffect(() => {
    setDeviceNameDraft(assetName);
  }, [assetName, nodeId]);

  const normalizedDeviceName = useMemo(() => deviceNameDraft.trim(), [deviceNameDraft]);
  const hasDeviceNameChange = useMemo(
    () => normalizedDeviceName !== assetName.trim(),
    [assetName, normalizedDeviceName]
  );

  const saveDeviceName = useCallback(async () => {
    if (normalizedDeviceName === "") {
      setDeviceNameError("Device name cannot be empty.");
      return;
    }
    if (!hasDeviceNameChange) {
      setDeviceNameMessage("No name changes to save.");
      return;
    }

    setSavingDeviceName(true);
    setDeviceNameError(null);
    setDeviceNameMessage(null);
    try {
      const response = await fetch(`/api/assets/${encodeURIComponent(nodeId)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: normalizedDeviceName }),
      });
      const responsePayload = await readJSON(response);
      if (!response.ok) {
        throw new Error(readError(responsePayload) || "failed to rename device");
      }
      const updatedName = readUpdatedAssetName(responsePayload) ?? normalizedDeviceName;
      setDeviceNameDraft(updatedName);
      setDeviceNameMessage("Device name updated.");
      await fetchStatus();
    } catch (err) {
      setDeviceNameError(err instanceof Error ? err.message : "failed to rename device");
    } finally {
      setSavingDeviceName(false);
    }
  }, [fetchStatus, hasDeviceNameChange, nodeId, normalizedDeviceName]);

  return {
    deviceNameDraft,
    setDeviceNameDraft,
    saveDeviceName,
    savingDeviceName,
    normalizedDeviceName,
    hasDeviceNameChange,
    deviceNameError,
    deviceNameMessage,
  };
}

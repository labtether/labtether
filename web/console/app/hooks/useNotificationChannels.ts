"use client";

import { useCallback, useEffect, useState } from "react";
import { sanitizeErrorMessage } from "../lib/sanitizeErrorMessage";

export type NotificationChannel = {
  id: string;
  name: string;
  type: string;
  config: Record<string, unknown>;
  enabled: boolean;
  created_at: string;
  updated_at: string;
};

type ChannelsPayload = {
  channels?: NotificationChannel[];
  error?: string;
};

type ChannelPayload = {
  error?: string;
};

type TestChannelPayload = {
  success: boolean;
  error?: string;
};

export function useNotificationChannels() {
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await fetch("/api/notifications/channels", { cache: "no-store", signal: AbortSignal.timeout(15_000) });
      const payload = (await response.json()) as ChannelsPayload;
      if (!response.ok) {
        throw new Error(payload.error || `failed to load notification channels (${response.status})`);
      }
      setChannels(payload.channels ?? []);
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "failed to load notification channels"));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const createChannel = useCallback(async (payload: Record<string, unknown>) => {
    const response = await fetch("/api/notifications/channels", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      signal: AbortSignal.timeout(15_000),
      body: JSON.stringify(payload),
    });
    const data = (await response.json()) as ChannelPayload;
    if (!response.ok) {
      throw new Error(data.error || `failed to create notification channel (${response.status})`);
    }
    await refresh();
  }, [refresh]);

  const updateChannel = useCallback(async (id: string, payload: Record<string, unknown>) => {
    const response = await fetch(`/api/notifications/channels/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      signal: AbortSignal.timeout(15_000),
      body: JSON.stringify(payload),
    });
    const data = (await response.json()) as ChannelPayload;
    if (!response.ok) {
      throw new Error(data.error || `failed to update notification channel (${response.status})`);
    }
    await refresh();
  }, [refresh]);

  const deleteChannel = useCallback(async (id: string) => {
    const response = await fetch(`/api/notifications/channels/${encodeURIComponent(id)}`, {
      method: "DELETE",
      signal: AbortSignal.timeout(15_000),
    });
    const data = (await response.json()) as ChannelPayload;
    if (!response.ok) {
      throw new Error(data.error || `failed to delete notification channel (${response.status})`);
    }
    await refresh();
  }, [refresh]);

  const toggleEnabled = useCallback(async (id: string, enabled: boolean) => {
    const response = await fetch(`/api/notifications/channels/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      signal: AbortSignal.timeout(15_000),
      body: JSON.stringify({ enabled }),
    });
    const data = (await response.json()) as ChannelPayload;
    if (!response.ok) {
      throw new Error(data.error || `failed to update notification channel (${response.status})`);
    }
    await refresh();
  }, [refresh]);

  const testChannel = useCallback(async (id: string): Promise<{ success: boolean; error?: string }> => {
    const response = await fetch(`/api/notifications/channels/${encodeURIComponent(id)}/test`, {
      method: "POST",
      signal: AbortSignal.timeout(20_000),
    });
    const data = (await response.json()) as TestChannelPayload;
    if (!response.ok) {
      return { success: false, error: data.error || `test request failed (${response.status})` };
    }
    return { success: data.success, error: data.error };
  }, []);

  return { channels, loading, error, refresh, createChannel, updateChannel, deleteChannel, toggleEnabled, testChannel };
}

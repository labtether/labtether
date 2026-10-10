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

export type NotificationChannelCapabilities = {
  smtp_insecure_transport_allowed: boolean;
};

type ChannelsPayload = {
  channels?: NotificationChannel[];
  capabilities?: Partial<NotificationChannelCapabilities>;
  error?: string;
};

type ChannelPayload = {
  error?: string;
};

type TestChannelPayload = {
  success?: boolean;
  error?: string;
};

const CHANNEL_PAGE_SIZE = 100;
const MAX_CHANNEL_PAGES = 100;

async function safePayload<T extends object>(response: Response): Promise<Partial<T>> {
  try {
    const value: unknown = await response.json();
    if (value && typeof value === "object" && !Array.isArray(value)) {
      return value as Partial<T>;
    }
  } catch {
    // Callers provide a bounded, non-sensitive fallback for malformed replies.
  }
  return {};
}

function safeChannelError(error: unknown, fallback: string): Error {
  const message = error instanceof Error ? error.message : "";
  return new Error(sanitizeErrorMessage(message, fallback));
}

export async function fetchAllNotificationChannels(): Promise<{
  channels: NotificationChannel[];
  capabilities: NotificationChannelCapabilities;
}> {
  const channels: NotificationChannel[] = [];
  const seenIDs = new Set<string>();
  let capabilities: NotificationChannelCapabilities | null = null;
  const signal = AbortSignal.timeout(15_000);

  for (let page = 0; page < MAX_CHANNEL_PAGES; page++) {
    const offset = page * CHANNEL_PAGE_SIZE;
    const response = await fetch(
      `/api/notifications/channels?limit=${CHANNEL_PAGE_SIZE}&offset=${offset}`,
      { cache: "no-store", signal },
    );
    const payload = await safePayload<ChannelsPayload>(response);
    if (!response.ok) {
      throw new Error(payload.error || `failed to load notification channels (${response.status})`);
    }
    const pageChannels = payload.channels;
    const insecureSMTPAllowed = payload.capabilities?.smtp_insecure_transport_allowed;
    if (
      !Array.isArray(pageChannels)
      || pageChannels.length > CHANNEL_PAGE_SIZE
      || typeof insecureSMTPAllowed !== "boolean"
      || (capabilities !== null && capabilities.smtp_insecure_transport_allowed !== insecureSMTPAllowed)
    ) {
      throw new Error(`incomplete notification channels response (offset ${offset})`);
    }
    for (const channel of pageChannels) {
      if (!channel || typeof channel.id !== "string" || !channel.id.trim() || seenIDs.has(channel.id)) {
        throw new Error(`incomplete notification channels response (offset ${offset})`);
      }
      seenIDs.add(channel.id);
    }
    capabilities ??= { smtp_insecure_transport_allowed: insecureSMTPAllowed };
    channels.push(...pageChannels);
    if (pageChannels.length < CHANNEL_PAGE_SIZE) return { channels, capabilities };
  }

  throw new Error("notification channels exceed the supported page limit");
}

export async function requestNotificationChannelTest(id: string): Promise<{ success: boolean; error?: string }> {
  try {
    const response = await fetch(`/api/notifications/channels/${encodeURIComponent(id)}/test`, {
      method: "POST",
      signal: AbortSignal.timeout(20_000),
    });
    const data = await safePayload<TestChannelPayload>(response);
    if (!response.ok) {
      return {
        success: false,
        error: sanitizeErrorMessage(data.error || "", `test request failed (${response.status})`),
      };
    }
    if (data.success !== true) {
      return {
        success: false,
        error: sanitizeErrorMessage(data.error || "", "test delivery was not confirmed"),
      };
    }
    return { success: true };
  } catch (error) {
    return {
      success: false,
      error: sanitizeErrorMessage(error instanceof Error ? error.message : "", "test request failed"),
    };
  }
}

export function useNotificationChannels() {
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [capabilities, setCapabilities] = useState<NotificationChannelCapabilities>({
    smtp_insecure_transport_allowed: false,
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = await fetchAllNotificationChannels();
      setChannels(result.channels);
      setCapabilities(result.capabilities);
    } catch (err) {
      setCapabilities({ smtp_insecure_transport_allowed: false });
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "failed to load notification channels"));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const createChannel = useCallback(async (payload: Record<string, unknown>) => {
    try {
      const response = await fetch("/api/notifications/channels", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        signal: AbortSignal.timeout(15_000),
        body: JSON.stringify(payload),
      });
      const data = await safePayload<ChannelPayload>(response);
      if (!response.ok) {
        throw new Error(data.error || `failed to create notification channel (${response.status})`);
      }
      await refresh();
    } catch (error) {
      throw safeChannelError(error, "failed to create notification channel");
    }
  }, [refresh]);

  const updateChannel = useCallback(async (id: string, payload: Record<string, unknown>) => {
    try {
      const response = await fetch(`/api/notifications/channels/${encodeURIComponent(id)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        signal: AbortSignal.timeout(15_000),
        body: JSON.stringify(payload),
      });
      const data = await safePayload<ChannelPayload>(response);
      if (!response.ok) {
        throw new Error(data.error || `failed to update notification channel (${response.status})`);
      }
      await refresh();
    } catch (error) {
      throw safeChannelError(error, "failed to update notification channel");
    }
  }, [refresh]);

  const deleteChannel = useCallback(async (id: string) => {
    try {
      const response = await fetch(`/api/notifications/channels/${encodeURIComponent(id)}`, {
        method: "DELETE",
        signal: AbortSignal.timeout(15_000),
      });
      const data = await safePayload<ChannelPayload>(response);
      if (!response.ok) {
        throw new Error(data.error || `failed to delete notification channel (${response.status})`);
      }
      await refresh();
    } catch (error) {
      throw safeChannelError(error, "failed to delete notification channel");
    }
  }, [refresh]);

  const toggleEnabled = useCallback(async (id: string, enabled: boolean) => {
    try {
      const response = await fetch(`/api/notifications/channels/${encodeURIComponent(id)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        signal: AbortSignal.timeout(15_000),
        body: JSON.stringify({ enabled }),
      });
      const data = await safePayload<ChannelPayload>(response);
      if (!response.ok) {
        throw new Error(data.error || `failed to update notification channel (${response.status})`);
      }
      await refresh();
    } catch (error) {
      throw safeChannelError(error, "failed to update notification channel");
    }
  }, [refresh]);

  const testChannel = useCallback(async (id: string): Promise<{ success: boolean; error?: string }> => {
    return requestNotificationChannelTest(id);
  }, []);

  return { channels, capabilities, loading, error, refresh, createChannel, updateChannel, deleteChannel, toggleEnabled, testChannel };
}

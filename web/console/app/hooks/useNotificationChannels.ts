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
      const response = await fetch("/api/notifications/channels", { cache: "no-store", signal: AbortSignal.timeout(15_000) });
      const payload = await safePayload<ChannelsPayload>(response);
      if (!response.ok) {
        throw new Error(payload.error || `failed to load notification channels (${response.status})`);
      }
      setChannels(payload.channels ?? []);
      setCapabilities({
        smtp_insecure_transport_allowed: payload.capabilities?.smtp_insecure_transport_allowed === true,
      });
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

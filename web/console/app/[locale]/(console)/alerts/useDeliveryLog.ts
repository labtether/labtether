"use client";

import { useCallback, useEffect, useState } from "react";
import { fetchAllNotificationChannels } from "../../../hooks/useNotificationChannels";
import { usePollingTask } from "../../../hooks/usePollingTask";
import { apiFetch } from "../../../lib/api";
import type { DeliveryRecord } from "./alertsPageTypes";

const AUTO_REFRESH_MS = 30_000;

/**
 * The hub's notification history (the latest 200), refreshed every 30 s, and the channels' names so rows can say
 * "Email" rather than a channel's ID. Names stay empty when the viewer may not list channels. Nothing is fetched
 * while enabled is false (the inbox has no alerts to show deliveries for).
 */
export function useDeliveryLog(enabled = true) {
  const [records, setRecords] = useState<DeliveryRecord[]>([]);
  const [channelNames, setChannelNames] = useState<Map<string, string>>(new Map());
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [failed, setFailed] = useState(false);

  const task = useCallback(async (signal: AbortSignal, manual: boolean) => {
    if (manual) setRefreshing(true);
    try {
      const { response, data } = await apiFetch<{ history?: DeliveryRecord[] }>("/api/notifications/history?limit=200", { signal });
      if (signal.aborted) return;
      if (!response.ok) {
        setFailed(true);
        return;
      }
      setFailed(false);
      setRecords(data?.history ?? []);
    } catch {
      if (!signal.aborted) setFailed(true);
    } finally {
      if (!signal.aborted) {
        setLoading(false);
        setRefreshing(false);
      }
    }
  }, []);

  const refresh = usePollingTask(task, AUTO_REFRESH_MS, enabled);

  useEffect(() => {
    if (!enabled) return;
    let live = true;
    fetchAllNotificationChannels()
      .then(({ channels }) => { if (live) setChannelNames(new Map(channels.map((c) => [c.id, c.name]))); })
      .catch(() => { /* Rows fall back to a plain "Unknown channel". */ });
    return () => { live = false; };
  }, [enabled]);

  return { records, channelNames, loading, refreshing, failed, refresh };
}

"use client";

import { useCallback, useEffect, useState } from "react";
import { apiFetch } from "../../../lib/api";
import { autoMutate } from "../actions/_parts/autoApi";
import { webhookRecordsFromPayload, type WebhookRecord } from "./webhookPayload";

export type WebhookInput = { name: string; url: string; secret: string; events: string[] };

const url = (id: string) => `/api/v2/webhooks/${encodeURIComponent(id)}`;
const body = (input: WebhookInput) => ({
  name: input.name,
  url: input.url,
  events: input.events,
  ...(input.secret ? { secret: input.secret } : {}),
});

/** The hub's webhooks and the writes the page makes. A secret is sent only when one was typed; the hub keeps the old one otherwise. */
export function useWebhooks() {
  const [webhooks, setWebhooks] = useState<WebhookRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);

  const load = useCallback(async () => {
    setFailed(false);
    try {
      const { response, data } = await apiFetch<unknown>("/api/v2/webhooks");
      if (!response.ok) throw new Error("load");
      setWebhooks(webhookRecordsFromPayload(data));
    } catch {
      setFailed(true);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const create = useCallback(async (input: WebhookInput) => {
    await autoMutate("/api/v2/webhooks", "POST", "", body(input));
    await load();
  }, [load]);

  const update = useCallback(async (webhook: WebhookRecord, input: WebhookInput) => {
    await autoMutate(url(webhook.id), "PATCH", "", { ...body(input), enabled: webhook.enabled });
    await load();
  }, [load]);

  const remove = useCallback(async (id: string) => {
    await autoMutate(url(id), "DELETE", "");
    setWebhooks((list) => list.filter((w) => w.id !== id));
    await load();
  }, [load]);

  /** Turns a webhook on or off at once in the list; puts it back when the hub refuses. */
  const setEnabled = useCallback(async (webhook: WebhookRecord, enabled: boolean) => {
    setWebhooks((list) => list.map((w) => (w.id === webhook.id ? { ...w, enabled } : w)));
    try {
      await autoMutate(url(webhook.id), "PATCH", "", { enabled });
    } catch (cause) {
      setWebhooks((list) => list.map((w) => (w.id === webhook.id ? { ...w, enabled: webhook.enabled } : w)));
      throw cause;
    }
    await load();
  }, [load]);

  return { webhooks, loading, failed, load, create, update, remove, setEnabled };
}

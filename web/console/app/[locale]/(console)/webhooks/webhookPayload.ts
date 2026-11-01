/** A webhook as the hub sends it. The hub never sends the secret back. */
export interface WebhookRecord {
  id: string;
  name: string;
  url: string;
  events: string[];
  enabled: boolean;
  last_triggered_at?: string | null;
  created_at?: string;
}

/** The webhook list from the hub's answer: its v2 envelope ({ data: [...] }), a bare array, or { webhooks: [...] }. */
export function webhookRecordsFromPayload(payload: unknown): WebhookRecord[] {
  if (Array.isArray(payload)) {
    return payload as WebhookRecord[];
  }
  if (!payload || typeof payload !== "object") {
    return [];
  }
  const record = payload as { data?: unknown; webhooks?: unknown };
  const list = Array.isArray(record.data) ? record.data : record.webhooks;
  return Array.isArray(list) ? (list as WebhookRecord[]) : [];
}

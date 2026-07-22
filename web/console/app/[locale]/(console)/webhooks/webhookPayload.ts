export interface WebhookRecord {
  id: string;
  name: string;
  url: string;
  secret?: string;
  events: string[];
  enabled: boolean;
  last_triggered_at?: string | null;
  created_at?: string;
}

interface WebhookEnvelope {
  webhooks?: unknown;
}

export function webhookRecordsFromPayload(payload: unknown): WebhookRecord[] {
  if (Array.isArray(payload)) {
    return payload as WebhookRecord[];
  }
  if (!payload || typeof payload !== "object") {
    return [];
  }
  const webhooks = (payload as WebhookEnvelope).webhooks;
  return Array.isArray(webhooks) ? (webhooks as WebhookRecord[]) : [];
}

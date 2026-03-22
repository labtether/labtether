"use client";

import { useCallback, useEffect, useState } from "react";
import { sanitizeErrorMessage } from "../lib/sanitizeErrorMessage";

export type ApiKeyInfo = {
  id: string;
  name: string;
  prefix: string;
  role: string;
  scopes: string[];
  allowed_assets?: string[];
  expires_at?: string | null;
  created_by: string;
  created_at: string;
  last_used_at?: string | null;
};

export type CreateKeyRequest = {
  name: string;
  role: string;
  scopes: string[];
  allowed_assets?: string[];
  expires_at?: string | null;
};

export type CreatedKeyResponse = ApiKeyInfo & {
  raw_key: string;
};

/* v2 API response envelopes — all responses wrapped in { request_id, data } or { error, message, status } */

type V2ListPayload = {
  data?: ApiKeyInfo[];
  meta?: { total: number; page: number; per_page: number };
  error?: string;
  message?: string;
};

type V2CreatePayload = {
  data?: CreatedKeyResponse;
  error?: string;
  message?: string;
};

type V2MutatePayload = {
  data?: { status?: string };
  error?: string;
  message?: string;
};

export function useApiKeys() {
  const [keys, setKeys] = useState<ApiKeyInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await fetch("/api/v2/keys", {
        cache: "no-store",
        signal: AbortSignal.timeout(15_000),
      });
      const payload = (await response.json().catch(() => ({}))) as V2ListPayload;
      if (!response.ok) {
        throw new Error(payload.message || payload.error || `failed to load API keys (${response.status})`);
      }
      setKeys(payload.data ?? []);
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "failed to load API keys"));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const createKey = useCallback(
    async (req: CreateKeyRequest): Promise<CreatedKeyResponse> => {
      const response = await fetch("/api/v2/keys", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        signal: AbortSignal.timeout(15_000),
        body: JSON.stringify(req),
      });
      const payload = (await response.json().catch(() => ({}))) as V2CreatePayload;
      if (!response.ok) {
        throw new Error(payload.message || payload.error || `failed to create API key (${response.status})`);
      }
      if (!payload.data) {
        throw new Error("API returned success but no key data");
      }
      await refresh();
      return payload.data;
    },
    [refresh],
  );

  const revokeKey = useCallback(
    async (id: string) => {
      const response = await fetch(`/api/v2/keys/${encodeURIComponent(id)}`, {
        method: "DELETE",
        signal: AbortSignal.timeout(15_000),
      });
      const payload = (await response.json().catch(() => ({}))) as V2MutatePayload;
      if (!response.ok) {
        throw new Error(payload.message || payload.error || `failed to revoke API key (${response.status})`);
      }
      await refresh();
    },
    [refresh],
  );

  return { keys, loading, error, refresh, createKey, revokeKey };
}

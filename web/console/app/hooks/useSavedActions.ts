"use client";

import { useCallback, useEffect, useState } from "react";

import { sanitizeErrorMessage } from "../lib/sanitizeErrorMessage";

export type SavedActionStep = {
  name: string;
  command: string;
  target: string;
};

export type SavedAction = {
  id: string;
  name: string;
  description?: string;
  steps: SavedActionStep[];
  created_by: string;
  created_at: string;
};

export type SavedActionRunStep = {
  name: string;
  target: string;
  exit_code?: number;
  output?: string;
  duration_ms?: number;
  error?: string;
  message?: string;
};

export type SavedActionRun = {
  action_id: string;
  steps: SavedActionRunStep[];
};

export type CreateSavedActionRequest = {
  name: string;
  description?: string;
  steps: SavedActionStep[];
};

type UnknownRecord = Record<string, unknown>;

function record(value: unknown): UnknownRecord | null {
  return value && typeof value === "object" && !Array.isArray(value) ? value as UnknownRecord : null;
}

function stringValue(value: unknown): string | null {
  return typeof value === "string" ? value : null;
}

function savedActionStep(value: unknown): SavedActionStep | null {
  const item = record(value);
  if (!item) return null;
  const name = stringValue(item.name);
  const command = stringValue(item.command);
  const target = stringValue(item.target);
  if (name === null || command === null || target === null) return null;
  return { name, command, target };
}

function savedAction(value: unknown): SavedAction | null {
  const item = record(value);
  if (!item || !Array.isArray(item.steps)) return null;
  const id = stringValue(item.id);
  const name = stringValue(item.name);
  const createdBy = stringValue(item.created_by);
  const createdAt = stringValue(item.created_at);
  const steps = item.steps.map(savedActionStep);
  if (id === null || name === null || createdBy === null || createdAt === null || steps.some((step) => step === null)) {
    return null;
  }
  const description = stringValue(item.description);
  return {
    id,
    name,
    ...(description ? { description } : {}),
    steps: steps as SavedActionStep[],
    created_by: createdBy,
    created_at: createdAt,
  };
}

function savedActionRunStep(value: unknown): SavedActionRunStep | null {
  const item = record(value);
  if (!item) return null;
  const name = stringValue(item.name);
  const target = stringValue(item.target);
  if (name === null || target === null) return null;
  return {
    name,
    target,
    ...(typeof item.exit_code === "number" ? { exit_code: item.exit_code } : {}),
    ...(typeof item.output === "string" ? { output: item.output } : {}),
    ...(typeof item.duration_ms === "number" ? { duration_ms: item.duration_ms } : {}),
    ...(typeof item.error === "string" ? { error: item.error } : {}),
    ...(typeof item.message === "string" ? { message: item.message } : {}),
  };
}

async function responsePayload(response: Response): Promise<UnknownRecord | null> {
  return record(await response.json().catch(() => null));
}

function responseError(response: Response, payload: UnknownRecord | null, fallback: string): Error {
  const raw = stringValue(payload?.message) || stringValue(payload?.error) || `${fallback} (${response.status})`;
  return new Error(sanitizeErrorMessage(raw, fallback));
}

export function useSavedActions() {
  const [actions, setActions] = useState<SavedAction[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadActions = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    setError(null);
    try {
      const response = await fetch("/api/v2/actions?limit=100", { cache: "no-store", signal });
      const payload = await responsePayload(response);
      if (!response.ok) throw responseError(response, payload, "Failed to load saved actions");
      const data = payload?.data;
      if (!Array.isArray(data)) throw new Error("Saved actions returned an invalid response");
      const parsed = data.map(savedAction);
      if (parsed.some((item) => item === null)) throw new Error("Saved actions returned an invalid response");
      setActions(parsed as SavedAction[]);
    } catch (cause) {
      if (cause instanceof DOMException && cause.name === "AbortError") return;
      const message = cause instanceof Error ? cause.message : "Failed to load saved actions";
      setError(sanitizeErrorMessage(message, "Failed to load saved actions"));
      throw cause;
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void loadActions(controller.signal).catch(() => undefined);
    return () => controller.abort();
  }, [loadActions]);

  const createAction = useCallback(async (request: CreateSavedActionRequest): Promise<SavedAction> => {
    const response = await fetch("/api/v2/actions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request),
      cache: "no-store",
    });
    const payload = await responsePayload(response);
    if (!response.ok) throw responseError(response, payload, "Failed to create saved action");
    const created = savedAction(payload?.data);
    if (!created) throw new Error("Saved action creation returned an invalid response");
    setActions((current) => [created, ...current.filter((item) => item.id !== created.id)]);
    return created;
  }, []);

  const getAction = useCallback(async (id: string): Promise<SavedAction> => {
    const response = await fetch(`/api/v2/actions/${encodeURIComponent(id)}`, { cache: "no-store" });
    const payload = await responsePayload(response);
    if (!response.ok) throw responseError(response, payload, "Failed to load saved action");
    const loaded = savedAction(payload?.data);
    if (!loaded) throw new Error("Saved action returned an invalid response");
    setActions((current) => current.map((item) => item.id === loaded.id ? loaded : item));
    return loaded;
  }, []);

  const runAction = useCallback(async (id: string): Promise<SavedActionRun> => {
    const response = await fetch(`/api/v2/actions/${encodeURIComponent(id)}/run`, {
      method: "POST",
      cache: "no-store",
    });
    const payload = await responsePayload(response);
    if (!response.ok) throw responseError(response, payload, "Failed to run saved action");
    const data = record(payload?.data);
    const actionID = stringValue(data?.action_id);
    if (!data || actionID === null || !Array.isArray(data.steps)) {
      throw new Error("Saved action run returned an invalid response");
    }
    const steps = data.steps.map(savedActionRunStep);
    if (steps.some((step) => step === null)) throw new Error("Saved action run returned an invalid response");
    return { action_id: actionID, steps: steps as SavedActionRunStep[] };
  }, []);

  const deleteAction = useCallback(async (id: string): Promise<void> => {
    const response = await fetch(`/api/v2/actions/${encodeURIComponent(id)}`, {
      method: "DELETE",
      cache: "no-store",
    });
    const payload = await responsePayload(response);
    if (!response.ok) throw responseError(response, payload, "Failed to delete saved action");
    const data = record(payload?.data);
    if (stringValue(data?.status) !== "deleted") throw new Error("Saved action deletion returned an invalid response");
    setActions((current) => current.filter((item) => item.id !== id));
  }, []);

  return {
    actions,
    loading,
    error,
    loadActions,
    createAction,
    getAction,
    runAction,
    deleteAction,
  };
}

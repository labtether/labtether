"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { AlertInstance, AlertRule, AlertRuleTemplate, AlertSilence } from "../console/models";

async function responseError(response: Response, fallback: string): Promise<Error> {
  const payload = await response.json().catch(() => null) as { error?: unknown } | null;
  const message = typeof payload?.error === "string" && payload.error.trim()
    ? payload.error.trim()
    : `${fallback} (HTTP ${response.status})`;
  return new Error(message);
}

export function useAlerts() {
  const [instances, setInstances] = useState<AlertInstance[]>([]);
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [templates, setTemplates] = useState<AlertRuleTemplate[]>([]);
  const [silences, setSilences] = useState<AlertSilence[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const mutationAbortRef = useRef<AbortController | null>(null);
  const mountedRef = useRef(true);

  const fetchAll = useCallback(async () => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    try {
      const [instRes, rulesRes, templatesRes, silencesRes] = await Promise.all([
        fetch("/api/alerts/instances", { cache: "no-store", signal: controller.signal }),
        fetch("/api/alerts/rules", { cache: "no-store", signal: controller.signal }),
        fetch("/api/alerts/templates", { cache: "no-store", signal: controller.signal }),
        fetch("/api/alerts/silences", { cache: "no-store", signal: controller.signal }),
      ]);

      const failedResponse = [instRes, rulesRes, templatesRes, silencesRes].find((response) => !response.ok);
      if (failedResponse) {
        throw await responseError(failedResponse, "alerts unavailable");
      }

      const [instanceData, ruleData, templateData, silenceData] = await Promise.all([
        instRes.json() as Promise<{ instances?: AlertInstance[] } | AlertInstance[]>,
        rulesRes.json() as Promise<{ rules?: AlertRule[] } | AlertRule[]>,
        templatesRes.json() as Promise<{ templates?: AlertRuleTemplate[] } | AlertRuleTemplate[]>,
        silencesRes.json() as Promise<{ silences?: AlertSilence[] } | AlertSilence[]>,
      ]);

      if (mountedRef.current) {
        setInstances(Array.isArray(instanceData) ? instanceData : instanceData.instances ?? []);
        setRules(Array.isArray(ruleData) ? ruleData : ruleData.rules ?? []);
        setTemplates(Array.isArray(templateData) ? templateData : templateData.templates ?? []);
        setSilences(Array.isArray(silenceData) ? silenceData : silenceData.silences ?? []);
        setError(null);
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      if (mountedRef.current) {
        setError(err instanceof Error ? err.message : "alerts unavailable");
      }
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      if (mountedRef.current) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    void fetchAll();
    return () => {
      mountedRef.current = false;
      abortRef.current?.abort();
      abortRef.current = null;
      mutationAbortRef.current?.abort();
      mutationAbortRef.current = null;
    };
  }, [fetchAll]);

  useEffect(() => {
    let refreshTimer: ReturnType<typeof setTimeout> | null = null;
    const scheduleRefresh = () => {
      if (document.visibilityState !== "visible") return;
      if (refreshTimer) clearTimeout(refreshTimer);
      refreshTimer = setTimeout(() => {
        refreshTimer = null;
        void fetchAll();
      }, 250);
    };
    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") scheduleRefresh();
    };

    window.addEventListener("labtether:alert-event", scheduleRefresh);
    document.addEventListener("visibilitychange", onVisibilityChange);
    const pollTimer = window.setInterval(scheduleRefresh, 30_000);

    return () => {
      window.removeEventListener("labtether:alert-event", scheduleRefresh);
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.clearInterval(pollTimer);
      if (refreshTimer) clearTimeout(refreshTimer);
    };
  }, [fetchAll]);

  const ackAlert = useCallback(async (id: string) => {
    mutationAbortRef.current?.abort();
    const controller = new AbortController();
    mutationAbortRef.current = controller;
    try {
      const response = await fetch("/api/alerts/instances", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id, action: "acknowledge" }),
        signal: controller.signal,
      });
      if (!response.ok) {
        const actionError = await responseError(response, "failed to acknowledge alert");
        await fetchAll();
        if (mountedRef.current) setError(actionError.message);
        return;
      }
      await fetchAll();
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") {
        return;
      }
      if (mountedRef.current) setError(err instanceof Error ? err.message : "failed to acknowledge alert");
    } finally {
      if (mutationAbortRef.current === controller) {
        mutationAbortRef.current = null;
      }
    }
  }, [fetchAll]);

  const resolveAlert = useCallback(async (id: string) => {
    mutationAbortRef.current?.abort();
    const controller = new AbortController();
    mutationAbortRef.current = controller;
    try {
      const response = await fetch("/api/alerts/instances", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id, action: "resolve" }),
        signal: controller.signal,
      });
      if (!response.ok) {
        const actionError = await responseError(response, "failed to resolve alert");
        await fetchAll();
        if (mountedRef.current) setError(actionError.message);
        return;
      }
      await fetchAll();
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") {
        return;
      }
      if (mountedRef.current) setError(err instanceof Error ? err.message : "failed to resolve alert");
    } finally {
      if (mutationAbortRef.current === controller) {
        mutationAbortRef.current = null;
      }
    }
  }, [fetchAll]);

  const createSilence = useCallback(async (payload: {
    matchers: Record<string, string>;
    starts_at: string;
    ends_at: string;
    reason?: string;
  }) => {
    mutationAbortRef.current?.abort();
    const controller = new AbortController();
    mutationAbortRef.current = controller;
    try {
      const res = await fetch("/api/alerts/silences", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
        signal: controller.signal,
      });
      if (!res.ok) {
        const body = await res.text();
        throw new Error(body || `HTTP ${res.status}`);
      }
      await fetchAll();
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") {
        return;
      }
      throw err;
    } finally {
      if (mutationAbortRef.current === controller) {
        mutationAbortRef.current = null;
      }
    }
  }, [fetchAll]);

  const deleteSilence = useCallback(async (id: string) => {
    mutationAbortRef.current?.abort();
    const controller = new AbortController();
    mutationAbortRef.current = controller;
    try {
      const res = await fetch(`/api/alerts/silences/${encodeURIComponent(id)}`, {
        method: "DELETE",
        signal: controller.signal,
      });
      if (!res.ok) {
        const body = await res.text();
        throw new Error(body || `HTTP ${res.status}`);
      }
      await fetchAll();
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") {
        return;
      }
      throw err;
    } finally {
      if (mutationAbortRef.current === controller) {
        mutationAbortRef.current = null;
      }
    }
  }, [fetchAll]);

  const createRule = useCallback(async (rule: Record<string, unknown>) => {
    mutationAbortRef.current?.abort();
    const controller = new AbortController();
    mutationAbortRef.current = controller;
    try {
      const res = await fetch("/api/alerts/rules", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(rule),
        signal: controller.signal,
      });
      if (!res.ok) {
        const payload = await res.json().catch(() => ({ error: "unknown error" })) as { error?: string };
        throw new Error(payload.error ?? "failed to create rule");
      }
      await fetchAll();
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") {
        return;
      }
      throw error;
    } finally {
      if (mutationAbortRef.current === controller) {
        mutationAbortRef.current = null;
      }
    }
  }, [fetchAll]);

  const deleteRule = useCallback(async (id: string) => {
    mutationAbortRef.current?.abort();
    const controller = new AbortController();
    mutationAbortRef.current = controller;
    try {
      const res = await fetch(`/api/alerts/rules/${encodeURIComponent(id)}`, { method: "DELETE", signal: controller.signal });
      if (!res.ok) {
        const payload = await res.json().catch(() => ({ error: "unknown error" })) as { error?: string };
        throw new Error(payload.error ?? "failed to delete rule");
      }
      await fetchAll();
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") {
        return;
      }
      throw error;
    } finally {
      if (mutationAbortRef.current === controller) {
        mutationAbortRef.current = null;
      }
    }
  }, [fetchAll]);

  return {
    instances,
    rules,
    templates,
    silences,
    loading,
    error,
    refresh: fetchAll,
    ackAlert,
    resolveAlert,
    createSilence,
    deleteSilence,
    createRule,
    deleteRule,
  };
}

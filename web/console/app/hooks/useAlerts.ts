"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { AlertInstance, AlertRule, AlertRuleTemplate, AlertSilence } from "../console/models";

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

      if (instRes.ok) {
        const data = (await instRes.json()) as { instances?: AlertInstance[] };
        if (mountedRef.current) {
          setInstances(data.instances ?? (Array.isArray(data) ? data as AlertInstance[] : []));
        }
      }
      if (rulesRes.ok) {
        const data = (await rulesRes.json()) as { rules?: AlertRule[] };
        if (mountedRef.current) {
          setRules(data.rules ?? (Array.isArray(data) ? data as AlertRule[] : []));
        }
      }
      if (templatesRes.ok) {
        const data = (await templatesRes.json()) as { templates?: AlertRuleTemplate[] };
        if (mountedRef.current) {
          setTemplates(data.templates ?? (Array.isArray(data) ? data as AlertRuleTemplate[] : []));
        }
      }
      if (silencesRes.ok) {
        const data = (await silencesRes.json()) as { silences?: AlertSilence[] };
        if (mountedRef.current) {
          setSilences(data.silences ?? (Array.isArray(data) ? data as AlertSilence[] : []));
        }
      }

      if (mountedRef.current) {
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

  const ackAlert = useCallback(async (id: string) => {
    mutationAbortRef.current?.abort();
    const controller = new AbortController();
    mutationAbortRef.current = controller;
    try {
      await fetch("/api/alerts/instances", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id, action: "acknowledge" }),
        signal: controller.signal,
      });
      await fetchAll();
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") {
        return;
      }
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
      await fetch("/api/alerts/instances", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id, action: "resolve" }),
        signal: controller.signal,
      });
      await fetchAll();
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") {
        return;
      }
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

"use client";

import { useEffect, useRef } from "react";
import { buildBrowserWsUrl } from "../../lib/ws";

// Full status includes heavier fields such as log aggregation.
const SLOW_POLL_MS = 120_000;
type RefreshStatus = (groupFilter: string) => Promise<void>;

export function useStatusRefresh(selectedGroupFilter: string, pollIntervalMs: number, fetchLiveStatus: RefreshStatus, fetchFullStatus: RefreshStatus) {
  // Group changes update requests without reconnecting the event stream.
  const groupFilterRef = useRef(selectedGroupFilter);
  useEffect(() => { groupFilterRef.current = selectedGroupFilter; }, [selectedGroupFilter]);

  // Fast polling loop (uses pollIntervalMs from runtime settings, default 5s)
  useEffect(() => {
    let timer: number | null = null;

    function startFastPolling() {
      stopFastPolling();
      void fetchLiveStatus(selectedGroupFilter);
      timer = window.setInterval(() => {
        void fetchLiveStatus(selectedGroupFilter);
      }, pollIntervalMs);
    }

    function stopFastPolling() {
      if (timer !== null) {
        window.clearInterval(timer);
        timer = null;
      }
    }

    function onVisibilityChange() {
      if (document.visibilityState === "visible") {
        startFastPolling();
      } else {
        stopFastPolling();
      }
    }

    if (document.visibilityState === "visible") {
      startFastPolling();
    }

    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      stopFastPolling();
    };
  }, [fetchLiveStatus, selectedGroupFilter, pollIntervalMs]);

  // Slow polling loop (120s, fixed — not affected by runtime poll interval setting)
  useEffect(() => {
    let timer: number | null = null;

    function startSlowPolling() {
      stopSlowPolling();
      void fetchFullStatus(selectedGroupFilter);
      timer = window.setInterval(() => {
        void fetchFullStatus(selectedGroupFilter);
      }, SLOW_POLL_MS);
    }

    function stopSlowPolling() {
      if (timer !== null) {
        window.clearInterval(timer);
        timer = null;
      }
    }

    function onVisibilityChange() {
      if (document.visibilityState === "visible") {
        startSlowPolling();
      } else {
        stopSlowPolling();
      }
    }

    if (document.visibilityState === "visible") {
      startSlowPolling();
    }

    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      stopSlowPolling();
    };
  }, [fetchFullStatus, selectedGroupFilter]);

  // Push events share one connection across group-filter changes.
  useEffect(() => {
    let visible = document.visibilityState === "visible";
    let socket: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let debounceTimer: ReturnType<typeof setTimeout> | null = null;
    let backoff = 2000;
    let lookupController: AbortController | null = null;
    let closed = false;

    function scheduleReconnect() {
      if (closed || !visible || reconnectTimer !== null) return;
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        backoff = Math.min(backoff * 2, 30_000);
        void connect();
      }, backoff);
    }

    async function resolveWsUrl(signal: AbortSignal): Promise<string | null> {
      const res = await fetch("/api/ws/events", { cache: "no-store", signal });
      if (!res.ok) return null;
      const data = (await res.json()) as { wsUrl?: string; streamPath?: string; secure?: boolean };
      if (data.wsUrl) return data.wsUrl;
      return data.streamPath ? buildBrowserWsUrl(data.streamPath, { secure: data.secure }) : null;
    }

    async function connect() {
      if (closed || !visible || lookupController !== null || socket !== null) return;
      const controller = new AbortController();
      lookupController = controller;
      try {
        const wsUrl = await resolveWsUrl(controller.signal);
        if (closed || !visible || controller.signal.aborted) return;
        if (!wsUrl) {
          scheduleReconnect();
          return;
        }
        const ws = new WebSocket(wsUrl);
        socket = ws;
        ws.onopen = () => { if (socket === ws) backoff = 2000; };
        ws.onmessage = (event) => {
          if (closed || !visible || socket !== ws) return;
          try {
            const msg = JSON.parse(event.data) as { type?: string };
            const pushTypes = ["alert.fired", "alert.resolved", "heartbeat.update", "job.completed", "agent.connected", "agent.disconnected"];
            if (!msg.type || !pushTypes.includes(msg.type)) return;
            if (msg.type.startsWith("alert.")) {
              window.dispatchEvent(new CustomEvent("labtether:alert-event", { detail: { type: msg.type } }));
            }
            if (debounceTimer !== null) clearTimeout(debounceTimer);
            debounceTimer = setTimeout(() => {
              debounceTimer = null;
              void fetchLiveStatus(groupFilterRef.current);
            }, 3000);
          } catch { /* Ignore malformed events. */ }
        };
        ws.onclose = () => {
          if (socket !== ws) return;
          socket = null;
          scheduleReconnect();
        };
        ws.onerror = () => { ws.close(); };
      } catch {
        // Polling remains available while the event endpoint is down.
        if (!controller.signal.aborted) scheduleReconnect();
      } finally {
        if (lookupController === controller) lookupController = null;
      }
    }

    function onVisibilityChange() {
      visible = document.visibilityState === "visible";
      if (visible) {
        void connect();
      } else {
        if (reconnectTimer !== null) clearTimeout(reconnectTimer);
        if (debounceTimer !== null) clearTimeout(debounceTimer);
        reconnectTimer = null;
        debounceTimer = null;
        lookupController?.abort();
        lookupController = null;
        // Hidden views ignore push events. Close their subscription so the hub
        // can stop serializing and sending those unused events as well.
        const previousSocket = socket;
        socket = null;
        previousSocket?.close();
      }
    }
    document.addEventListener("visibilitychange", onVisibilityChange);
    void connect();

    return () => {
      closed = true;
      document.removeEventListener("visibilitychange", onVisibilityChange);
      if (reconnectTimer !== null) clearTimeout(reconnectTimer);
      if (debounceTimer !== null) clearTimeout(debounceTimer);
      lookupController?.abort();
      socket?.close();
    };
  }, [fetchLiveStatus]);
}

"use client";

import { useEffect, useRef } from "react";
import { useConnectedAgents } from "./useConnectedAgents";
import { useToast } from "../contexts/ToastContext";

/**
 * Shows toast notifications when agents connect or disconnect.
 * Uses polling-based diff from useConnectedAgents (10s cycle).
 * Skips the first two state changes (initial empty default + first API response)
 * to establish a real baseline before diffing.
 */
export function usePresenceToasts() {
  const { connectedAgentIds } = useConnectedAgents();
  const { addToast } = useToast();
  const prevIds = useRef<Set<string> | null>(null);
  const warmup = useRef(true);

  useEffect(() => {
    if (prevIds.current === null) {
      // First render — connectedAgentIds is the empty initial default.
      // Snapshot it but don't diff yet.
      prevIds.current = new Set(connectedAgentIds);
      return;
    }

    if (warmup.current) {
      // Second change — first real API response arrived.
      // Use this as the true baseline, skip diffing.
      prevIds.current = new Set(connectedAgentIds);
      warmup.current = false;
      return;
    }

    const prev = prevIds.current;
    const current = connectedAgentIds;

    // New agents
    for (const id of current) {
      if (!prev.has(id)) {
        addToast("success", `Agent connected: ${id}`);
      }
    }

    // Departed agents
    for (const id of prev) {
      if (!current.has(id)) {
        addToast("warning", `Agent disconnected: ${id}`);
      }
    }

    prevIds.current = new Set(current);
  }, [connectedAgentIds, addToast]);
}

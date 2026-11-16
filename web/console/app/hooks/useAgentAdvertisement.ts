"use client";

import type { AgentAdvertisement, AgentAdvertisements } from "../console/agentOffers";
import { useFastStatus } from "../contexts/StatusContext";

const NONE: AgentAdvertisements = Object.freeze({});

/** Every device's advertisement from the latest status; empty when no agent lists what it offers. */
export function useAgentAdvertisements(): AgentAdvertisements {
  return useFastStatus()?.agentAdvertisements ?? NONE;
}

/** One device's advertisement, or null (today's rules apply). */
export function useAgentAdvertisement(assetId: string): AgentAdvertisement | null {
  return useAgentAdvertisements()[assetId] ?? null;
}

"use client";

import { useConnectedAgentsContext } from "../contexts/ConnectedAgentsContext";

/**
 * Returns the set of asset IDs with a connected agent and a manual refresh.
 * Reads from shared ConnectedAgentsProvider context (single 10s poll for all consumers).
 */
export function useConnectedAgents() {
  return useConnectedAgentsContext();
}

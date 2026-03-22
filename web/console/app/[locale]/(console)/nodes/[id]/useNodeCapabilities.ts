"use client";

import { useMemo } from "react";

type NetworkMethodOption = {
  value: string;
  label: string;
};

type UseNodeCapabilitiesArgs = {
  metadata: Record<string, string>;
  nodeHasAgent: boolean;
  nodePlatform: string;
};

type UseNodeCapabilitiesResult = {
  isLinuxAgentNode: boolean;
  supportsServiceListing: boolean;
  supportsPackageListing: boolean;
  supportsScheduleListing: boolean;
  supportsNetworkListing: boolean;
  supportsNetworkActions: boolean;
  supportsLogQuery: boolean;
  logQueryModeLabel: string;
  nodeNetworkMethodOptions: NetworkMethodOption[];
  nodeNetworkControlsLabel: string;
  nodeNetworkControlsHint: string;
};

export function supportsCapability(
  metadata: Record<string, string>,
  capabilityKey: string,
  capability: string,
  legacyDefault: boolean,
): boolean {
  const raw = metadata[capabilityKey];
  if (typeof raw !== "string" || raw.trim() === "") {
    return legacyDefault;
  }
  const values = new Set(
    raw
      .split(",")
      .map((value) => value.trim().toLowerCase())
      .filter((value) => value !== ""),
  );
  return values.has(capability.toLowerCase());
}

export function useNodeCapabilities({
  metadata,
  nodeHasAgent,
  nodePlatform,
}: UseNodeCapabilitiesArgs): UseNodeCapabilitiesResult {
  const isLinuxAgentNode = nodeHasAgent && nodePlatform === "linux";
  const supportsServiceListing = nodeHasAgent && supportsCapability(metadata, "cap_services", "list", true);
  const supportsPackageListing = nodeHasAgent && supportsCapability(metadata, "cap_packages", "list", true);
  const supportsScheduleListing = nodeHasAgent && supportsCapability(metadata, "cap_schedules", "list", true);
  const supportsNetworkListing = nodeHasAgent && supportsCapability(metadata, "cap_network", "list", true);
  const supportsNetworkActions = nodeHasAgent && supportsCapability(metadata, "cap_network", "action", isLinuxAgentNode);
  const supportsLogQuery = nodeHasAgent && supportsCapability(metadata, "cap_logs", "query", true);

  const networkActionBackend = (metadata["network_action_backend"] ?? "").trim().toLowerCase();
  const logQueryModeLabel = metadata["log_backend"] === "oslog" ? "System Log" : "Journal";

  const nodeNetworkMethodOptions = useMemo(() => {
    if (!supportsNetworkActions) {
      return [] as NetworkMethodOption[];
    }
    switch (networkActionBackend) {
      case "networksetup":
        return [
          { value: "auto", label: "Method: Auto" },
          { value: "networksetup", label: "Method: NetworkSetup" },
        ];
      case "nmcli":
        return [
          { value: "auto", label: "Method: Auto" },
          { value: "nmcli", label: "Method: NMCLI" },
        ];
      case "netplan":
      default:
        if (!isLinuxAgentNode && networkActionBackend !== "netplan") {
          return [{ value: "auto", label: "Method: Auto" }];
        }
        return [
          { value: "auto", label: "Method: Auto" },
          { value: "netplan", label: "Method: Netplan" },
          { value: "nmcli", label: "Method: NMCLI" },
        ];
    }
  }, [isLinuxAgentNode, networkActionBackend, supportsNetworkActions]);

  const nodeNetworkControlsLabel = networkActionBackend === "networksetup"
    ? "Network Controls (macOS)"
    : "Network Controls";
  const nodeNetworkControlsHint = networkActionBackend === "networksetup"
    ? "Auto method uses networksetup and can target a specific network service."
    : networkActionBackend === "nmcli"
      ? "Auto method uses nmcli."
      : "Auto method prefers netplan, then falls back to nmcli.";

  return {
    isLinuxAgentNode,
    supportsServiceListing,
    supportsPackageListing,
    supportsScheduleListing,
    supportsNetworkListing,
    supportsNetworkActions,
    supportsLogQuery,
    logQueryModeLabel,
    nodeNetworkMethodOptions,
    nodeNetworkControlsLabel,
    nodeNetworkControlsHint,
  };
}

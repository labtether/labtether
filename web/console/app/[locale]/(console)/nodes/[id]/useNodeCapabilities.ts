"use client";

import { useMemo } from "react";
import { deviceAgentOffers, type AgentAdvertisement, type DeviceAgentOffers } from "../../../../console/agentOffers";

type NetworkMethodOption = {
  value: string;
  label: string;
};

type UseNodeCapabilitiesArgs = {
  metadata: Record<string, string>;
  nodeHasAgent: boolean;
  nodePlatform: string;
  /** The Hub's agentAdvertisements entry for this device, if any. */
  advertisement?: AgentAdvertisement | null;
};

type UseNodeCapabilitiesResult = {
  /** Every agent feature this device offers (console/agentOffers); the flags below are read from it. */
  agentOffers: DeviceAgentOffers;
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

/** The device log's own name: macOS System Log, Windows Event Log, else the systemd journal. */
export function logQueryModeLabelFor(logBackend: string | undefined): string {
  if (logBackend === "oslog") return "System Log";
  if (logBackend === "eventlog") return "Event Log";
  return "Journal";
}

export function useNodeCapabilities({
  metadata,
  nodeHasAgent,
  nodePlatform,
  advertisement = null,
}: UseNodeCapabilitiesArgs): UseNodeCapabilitiesResult {
  const isLinuxAgentNode = nodeHasAgent && nodePlatform === "linux";
  const agentOffers = useMemo(
    () => deviceAgentOffers({ metadata, hasAgent: nodeHasAgent, platform: nodePlatform, advertisement }),
    [metadata, nodeHasAgent, nodePlatform, advertisement],
  );
  const supportsServiceListing = agentOffers.services;
  const supportsPackageListing = agentOffers.packages;
  const supportsScheduleListing = agentOffers.schedules;
  const supportsNetworkListing = agentOffers.network;
  const supportsNetworkActions = agentOffers.networkActions;
  const supportsLogQuery = agentOffers.logQuery;

  const networkActionBackend = (metadata["network_action_backend"] ?? "").trim().toLowerCase();
  const logQueryModeLabel = logQueryModeLabelFor(metadata["log_backend"]);

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
      : "For Netplan, save a snapshot before editing /etc/netplan. Then apply. Rollback restores those saved files. Auto prefers Netplan, then NMCLI.";

  return {
    agentOffers,
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

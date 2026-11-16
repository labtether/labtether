// Device fixtures for the agent-offers tests. Host names are made up.
import type { AgentAdvertisement } from "../agentOffers";
import type { Asset } from "../models";

// Heartbeat metadata the released Go agent sends with the usual tools present: the same values the Hub pins in
// internal/hubapi/resources/agent_capability_set_test.go (goAgentMetadata), from labtether-agent
// internal/agentplatform/*/provider_metadata.go. Go agents send no agent_capability_mode.
export const goAgentMetadata: Record<"linux" | "windows" | "darwin", Record<string, string>> = {
  linux: {
    cap_services: "list,action", service_backend: "systemd",
    cap_packages: "list,action", package_backend: "apt",
    cap_logs: "stored,query,stream", log_backend: "journalctl",
    cap_schedules: "list",
    cap_network: "list,action", network_backend: "ifconfig", network_action_backend: "nmcli",
  },
  windows: {
    cap_services: "list,action", service_backend: "scm",
    cap_logs: "stored,query,stream", log_backend: "eventlog",
    cap_schedules: "list", cron_backend: "taskscheduler",
    cap_network: "list,action", network_backend: "netsh",
    cap_packages: "list,action", package_backend: "registry", package_action_backend: "winget",
  },
  darwin: {
    cap_services: "list,action", service_backend: "launchd",
    cap_packages: "list,action", package_backend: "brew",
    cap_logs: "stored,query,stream", log_backend: "oslog",
    cap_schedules: "list",
    cap_network: "list,action", network_backend: "networksetup", network_action_backend: "networksetup",
  },
};

// The monitoring edition (Mac App Store): what the Hub reported for its lab stand-in in the Hub step's lab check.
export const monitoringMetadata: Record<string, string> = {
  agent_capability_mode: "explicit",
  agent_edition: "monitoring",
  cap_disks: "list",
  cap_network: "list",
  network_backend: "ifconfig",
  network_action_backend: "none",
  agent_capabilities: "",
};
export const monitoringAd: AgentAdvertisement = { explicit: true, edition: "monitoring", offered: ["disk.list", "network.list"] };

// An explicit agent that offers more: terminal and files, process and disk lists, but no power and no services.
export const partialMetadata: Record<string, string> = {
  agent_capability_mode: "explicit",
  agent_capabilities: "files,terminal",
  cap_disks: "list",
  cap_processes: "list",
  cap_network: "list",
};
export const partialAd: AgentAdvertisement = {
  explicit: true,
  edition: "",
  offered: [
    "disk.list", "file.copy", "file.delete", "file.list", "file.mkdir", "file.read", "file.rename", "file.search",
    "file.write", "network.list", "process.list", "terminal.close", "terminal.data", "terminal.probe",
    "terminal.resize", "terminal.start", "terminal.tmux.kill",
  ],
};

export function agentAsset(id: string, platform: string, metadata: Record<string, string>): Asset {
  return {
    id,
    name: id,
    type: "host",
    source: "agent",
    status: "online",
    platform,
    last_seen_at: "2026-11-16T10:00:00Z",
    metadata: { platform, ...metadata },
  };
}

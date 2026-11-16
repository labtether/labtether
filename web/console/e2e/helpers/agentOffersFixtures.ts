import { type Page } from "@playwright/test";
import { buildLiveStatusPayload, buildStatusPayload, installConsoleApiMocks } from "./consoleApiMocks";

// Three connected agent devices with made-up names: the monitoring edition (explicit, disk and network lists only),
// an explicit agent that offers terminal, files and some lists but no power, and a Go agent that sends no signal.
// Advertisements match what the Hub reports for this metadata (internal/agentcaps).

const NOW = "2026-11-16T10:00:00.000Z";

function agentDevice(id: string, platform: string, osName: string, ip: string, metadata: Record<string, string>) {
  return {
    id,
    name: id,
    type: "host",
    source: "agent",
    status: "online",
    platform,
    last_seen_at: NOW,
    metadata: { platform, os_name: osName, ip_address: ip, agent_version: "2.1.0", cpu_cores_physical: "8", memory_total_bytes: "17179869184", ...metadata },
  };
}

export const MONITORING_ID = "mac-mon-01";
export const EXPLICIT_ID = "rust-node-01";
export const GO_ID = "web-01";

export const agentOffersAssets = [
  agentDevice(MONITORING_ID, "darwin", "macOS 15.4", "192.0.2.21", {
    agent_capability_mode: "explicit", agent_edition: "monitoring", cap_disks: "list", cap_network: "list",
    network_backend: "ifconfig", network_action_backend: "none", agent_capabilities: "",
  }),
  agentDevice(EXPLICIT_ID, "linux", "Debian GNU/Linux 13", "192.0.2.22", {
    agent_capability_mode: "explicit", agent_capabilities: "files,terminal", cap_disks: "list", cap_processes: "list", cap_network: "list",
  }),
  agentDevice(GO_ID, "linux", "Ubuntu 24.04 LTS", "192.0.2.23", {
    cap_services: "list,action", service_backend: "systemd", cap_packages: "list,action", package_backend: "apt",
    cap_logs: "stored,query,stream", log_backend: "journalctl", cap_schedules: "list",
    cap_network: "list,action", network_backend: "ifconfig", network_action_backend: "nmcli",
  }),
];

export const agentAdvertisements = {
  [MONITORING_ID]: { explicit: true, edition: "monitoring", offered: ["disk.list", "network.list"] },
  [EXPLICIT_ID]: {
    explicit: true,
    offered: [
      "disk.list", "file.copy", "file.delete", "file.list", "file.mkdir", "file.read", "file.rename", "file.search", "file.write",
      "network.list", "process.list", "terminal.close", "terminal.data", "terminal.probe", "terminal.resize", "terminal.start",
      "terminal.tmux.kill",
    ],
  },
};

/** Console API mocks for the three devices; withAdvertisements false gives the status a Hub without this change sends. */
export async function installAgentOffersMocks(page: Page, withAdvertisements = true) {
  const ids = agentOffersAssets.map((asset) => asset.id);
  const tab = { id: "tab-default", name: "Default", layout: "single", panes: [], sort_order: 0 };
  let created = false;
  const extra = withAdvertisements ? { agentAdvertisements } : {};
  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets: agentOffersAssets, ...extra }),
    liveStatusPayload: buildLiveStatusPayload({ assets: agentOffersAssets, ...extra }),
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === "/api/agents/connected") {
        await fulfillJSON({ assets: ids, assetsInfo: ids.map((id) => ({ id, has_tmux: false })) });
        return true;
      }
      // The Terminal page: no saved tabs, so it creates one and shows its start page (as terminal-workspace.spec.ts).
      if (pathname === "/api/terminal/preferences" && method === "GET") return fulfillJSON({ preferences: {} }).then(() => true);
      if (pathname === "/api/terminal/snippets" && method === "GET") return fulfillJSON({ snippets: [] }).then(() => true);
      if (pathname === "/api/terminal/workspace/tabs" && method === "GET") return fulfillJSON({ tabs: created ? [tab] : [] }).then(() => true);
      if (pathname === "/api/terminal/workspace/tabs" && method === "POST") {
        created = true;
        await fulfillJSON({ tab });
        return true;
      }
      return false;
    },
  });
}

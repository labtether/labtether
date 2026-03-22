import { CATEGORIES } from "../../../../console/taxonomy";
import type { CategoryDef, CategorySlug } from "../../../../console/taxonomy";

export type InfraCategoryInfo = Map<CategorySlug, { def: CategoryDef; count: number }>;

export type BuildAvailableTabsInput = {
  templateTabs?: string[];
  isDockerContainer: boolean;
  isDockerStack: boolean;
  isInfra: boolean;
  isDockerHost: boolean;
  isProxmoxAsset: boolean;
  isTrueNASAsset: boolean;
  isPBSAsset: boolean;
  effectiveKind: string;
  infraCategories: InfraCategoryInfo;
  nodeHasAgent: boolean;
  isLinuxAgentNode: boolean;
  supportsServiceListing: boolean;
  supportsPackageListing: boolean;
  supportsScheduleListing: boolean;
  supportsNetworkListing: boolean;
  /** True when a Docker container-host is merged into this agent host. */
  hasMergedDockerHost?: boolean;
};

export function buildAvailableTabs({
  templateTabs,
  isDockerContainer,
  isDockerStack,
  isInfra,
  isDockerHost,
  isProxmoxAsset,
  isTrueNASAsset,
  isPBSAsset,
  effectiveKind,
  infraCategories,
  nodeHasAgent,
  isLinuxAgentNode,
  supportsServiceListing,
  supportsPackageListing,
  supportsScheduleListing,
  supportsNetworkListing,
  hasMergedDockerHost,
}: BuildAvailableTabsInput): string[] {
  // Docker assets always get first-class fixed tabs. Template tab bindings
  // should not be able to hide container terminal/logs/stats/inspect flows.
  if (isDockerContainer) {
    return ["overview", "terminal", "logs", "stats", "inspect"];
  }
  if (isDockerStack) {
    return ["overview", "containers", "inspect"];
  }
  if (isInfra && isDockerHost) {
    return ["overview", "containers", "images", "stacks", "terminal"];
  }

  const normalizedTemplateTabs = normalizeTemplateTabs(templateTabs ?? []);
  if (normalizedTemplateTabs.length > 0) {
    const tabs = [...normalizedTemplateTabs];
    if (nodeHasAgent) {
      insertTabAfter(tabs, "files", "packages");
      insertTabAfter(tabs, "agent-settings", "files");
    }
    if (hasMergedDockerHost) {
      insertTabAfter(tabs, "containers", "agent-settings");
      insertTabAfter(tabs, "stacks", "containers");
      insertTabAfter(tabs, "images", "stacks");
    }
    if (shouldIncludeDesktopTab({ isDockerContainer, isInfra, isDockerHost, isProxmoxAsset, effectiveKind })) {
      insertTabAfter(tabs, "desktop", "terminal");
    }
    return tabs;
  }

  if (isInfra) {
    // Infrastructure hosts: Overview + category tabs + action tabs
    const tabs: string[] = ["overview"];
    // Add non-empty category tabs in CATEGORIES order
    for (const cat of CATEGORIES) {
      if (infraCategories.has(cat.slug)) {
        tabs.push(cat.slug);
      }
    }
    if (isTrueNASAsset) {
      tabs.push("truenas");
    }
    if (isPBSAsset) {
      tabs.push("pbs");
    }
    // Agent-gated tabs
    if (nodeHasAgent) {
      tabs.push("processes");
      if (supportsServiceListing) {
        tabs.push("services");
      }
      if (supportsPackageListing) {
        tabs.push("packages");
      }
      tabs.push("files");
      tabs.push("agent-settings");
      if (supportsNetworkListing) {
        tabs.push("interfaces");
      }
      if (supportsScheduleListing) {
        tabs.push("cron");
      }
      if (isLinuxAgentNode) {
        tabs.push("disks");
        tabs.push("users");
      }
    }
    // Action tabs based on capabilities
    tabs.push("terminal");
    if (isProxmoxAsset && effectiveKind === "node") {
      tabs.push("desktop");
      tabs.push("settings");
    }
    return tabs;
  }
  // Non-infra assets: keep existing tab logic
  const tabs: string[] = ["overview"];
  if (isTrueNASAsset) {
    tabs.splice(1, 0, "truenas");
  }
  if (isPBSAsset) {
    tabs.splice(1, 0, "pbs");
  }
  if (nodeHasAgent) {
    tabs.push("processes");
    if (supportsServiceListing) {
      tabs.push("services");
    }
    if (supportsPackageListing) {
      tabs.push("packages");
    }
    tabs.push("files");
    tabs.push("agent-settings");
    if (supportsNetworkListing) {
      tabs.push("interfaces");
    }
    if (supportsScheduleListing) {
      tabs.push("cron");
    }
    if (isLinuxAgentNode) {
      tabs.push("disks");
      tabs.push("users");
    }
  }
  if (hasMergedDockerHost) {
    tabs.push("containers", "stacks", "images");
  }
  tabs.push("telemetry", "logs", "actions");
  if (isProxmoxAsset) {
    tabs.splice(1, 0, "proxmox");
  }
  if (!isProxmoxAsset || effectiveKind !== "storage") {
    tabs.push("terminal");
  }
  if (shouldIncludeDesktopTab({ isDockerContainer, isInfra, isDockerHost, isProxmoxAsset, effectiveKind })) {
    tabs.push("desktop");
  }
  if (isProxmoxAsset && effectiveKind === "node") {
    tabs.push("settings");
  }
  return tabs;
}

function normalizeTemplateTabs(tabs: string[]): string[] {
  if (tabs.length === 0) return [];
  const allowed = new Set<string>([
    "overview",
    "proxmox",
    "truenas",
    "pbs",
    "telemetry",
    "logs",
    "actions",
    "processes",
    "services",
    "packages",
    "files",
    "agent-settings",
    "disks",
    "interfaces",
    "users",
    "cron",
    "terminal",
    "desktop",
    "settings",
    "containers",
    "images",
    "stacks",
    "stats",
    "inspect",
    ...CATEGORIES.map((category) => category.slug),
  ]);

  const cleaned: string[] = [];
  const seen = new Set<string>();
  for (const rawTab of tabs) {
    const tab = rawTab.trim().toLowerCase();
    if (!tab || !allowed.has(tab) || seen.has(tab)) continue;
    seen.add(tab);
    cleaned.push(tab);
  }

  if (cleaned.length === 0) return [];
  if (!seen.has("overview")) {
    return ["overview", ...cleaned];
  }
  if (cleaned[0] === "overview") {
    return cleaned;
  }
  return ["overview", ...cleaned.filter((tab) => tab !== "overview")];
}

type DesktopTabEligibilityInput = Pick<
  BuildAvailableTabsInput,
  "isDockerContainer" | "isInfra" | "isDockerHost" | "isProxmoxAsset" | "effectiveKind"
>;

function shouldIncludeDesktopTab({
  isDockerContainer,
  isInfra,
  isDockerHost,
  isProxmoxAsset,
  effectiveKind,
}: DesktopTabEligibilityInput): boolean {
  if (isDockerContainer) return false;
  if (isInfra) {
    if (isDockerHost) return false;
    return isProxmoxAsset && effectiveKind === "node";
  }
  return !isProxmoxAsset || effectiveKind === "qemu" || effectiveKind === "lxc";
}

function insertTabAfter(tabs: string[], tab: string, after: string): void {
  if (tabs.includes(tab)) return;
  const index = tabs.indexOf(after);
  if (index === -1) {
    tabs.push(tab);
    return;
  }
  tabs.splice(index + 1, 0, tab);
}

export function nodeDetailTabLabel(tab: string, infraCategories: InfraCategoryInfo): string {
  // Static tab labels
  const staticLabels: Record<string, string> = {
    overview: "Overview",
    proxmox: "Proxmox",
    truenas: "TrueNAS",
    pbs: "PBS",
    telemetry: "Metrics",
    logs: "Logs",
    actions: "Actions",
    processes: "Processes",
    services: "Services",
    packages: "Packages",
    files: "Files",
    "agent-settings": "Agent Settings",
    disks: "Disks",
    interfaces: "Interfaces",
    users: "Users",
    cron: "Cron / Timers",
    terminal: "Terminal",
    desktop: "Remote View",
    settings: "Settings",
    containers: "Containers",
    images: "Images",
    stacks: "Stacks",
    stats: "Stats",
    inspect: "Inspect",
  };
  if (staticLabels[tab]) return staticLabels[tab];
  // Category tabs: show label + count
  const catInfo = infraCategories.get(tab as CategorySlug);
  if (catInfo) return `${catInfo.def.label} (${catInfo.count})`;
  return tab;
}

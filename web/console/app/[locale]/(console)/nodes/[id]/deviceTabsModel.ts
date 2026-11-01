// The device page's tab row. The open panel stays in the address (?panel=…) so Back and shared links return to it;
// older panel names keep working because each one belongs to a tab: Overview, what the device holds (its
// integration's tabs), System, Logs and Settings. Terminal, Files and Remote view open from the header and show as an
// extra tab only while they are open.

/** A tab: its key, the panel its link opens (null = Overview) and the words for its label. */
export type DeviceTab = {
  key: string;
  panel: string | null;
  /** A key under devices.page.tabs, or a plain label from the integration. */
  labelKey?: string;
  label?: string;
};

/** System views in switch order; "system" is the hardware view, "monitoring" the metrics history. */
export const SYSTEM_VIEWS = ["processes", "network", "services", "cron", "packages", "disks", "users", "system", "monitoring"] as const;
export const LOGS_VIEWS = ["logs", "actions"] as const;
export const SETTINGS_VIEWS = ["agent-settings", "settings", "connect"] as const;
/** Opened from the header and the ⋯ menu; a tab only while open. */
export const CONNECT_VIEWS = ["terminal", "files", "desktop"] as const;

/** What a device holds, in tab order. Infra categories (compute, storage, …) slot in after Proxmox. */
const INTEGRATIONS = ["proxmox", "docker", "portainer", "homeassistant", "truenas", "pbs"];
/** Docker asset views (a container or a compose stack), which have no panel card of their own. */
const DOCKER_CONTAINER_TABS = ["stats", "inspect"];
const DOCKER_STACK_TABS = ["containers", "inspect"];

const INTEGRATION_LABEL: Record<string, string> = {
  proxmox: "proxmox",
  docker: "docker",
  portainer: "portainer",
  homeassistant: "homeassistant",
  truenas: "truenas",
  pbs: "backups",
  stats: "stats",
  inspect: "inspect",
  containers: "containers",
  compute: "compute",
  storage: "storage",
  network: "network",
  services: "services",
  power: "power",
  terminal: "terminal",
  files: "files",
  desktop: "desktop",
};

export type DeviceTabsInput = {
  /** Panel ids the role may open, in the order the device offers them. */
  panelIds: string[];
  /** Infra category slugs this host has (compute, storage, …): their panels are category lists. */
  categorySlugs: Set<string>;
  isDockerContainer: boolean;
  isDockerStack: boolean;
  isProxmoxHost: boolean;
  activePanel: string | null;
};

const has = (list: readonly string[], id: string | null): boolean => id !== null && list.includes(id);

/** The tab a panel belongs to. Category slugs win over System view names (an infra host's "network" is a list). */
export function tabOfPanel(panel: string | null, categorySlugs: Set<string>): string {
  if (!panel) return "overview";
  if (categorySlugs.has(panel)) return panel;
  if (has(SYSTEM_VIEWS, panel)) return "system";
  if (has(LOGS_VIEWS, panel)) return "logs";
  if (has(SETTINGS_VIEWS, panel)) return "settings";
  return panel;
}

/** The System views this device offers, in switch order. */
export function systemViewsOf(panelIds: string[], categorySlugs: Set<string>): string[] {
  return SYSTEM_VIEWS.filter((id) => panelIds.includes(id) && !categorySlugs.has(id));
}

/** The Logs views this device offers. */
export function logsViewsOf(panelIds: string[]): string[] {
  return LOGS_VIEWS.filter((id) => panelIds.includes(id));
}

function labelFor(key: string, input: DeviceTabsInput): Pick<DeviceTab, "labelKey" | "label"> {
  if (key === "compute" && input.isProxmoxHost) return { labelKey: "guests" };
  const labelKey = INTEGRATION_LABEL[key];
  return labelKey ? { labelKey } : { label: key.charAt(0).toUpperCase() + key.slice(1).replace(/-/g, " ") };
}

/**
 * The tab row: Overview, then what the device holds (integration and category tabs), System, Logs and Settings.
 * An open panel that has no tab of its own (Terminal, Files, Remote view, or a panel the device does not list)
 * is added at the end so the row always shows where the page is.
 */
export function buildDeviceTabs(input: DeviceTabsInput): DeviceTab[] {
  const { panelIds, categorySlugs, isDockerContainer, isDockerStack, activePanel } = input;
  const tabs: DeviceTab[] = [{ key: "overview", panel: null, labelKey: "overview" }];
  const add = (key: string, panel: string = key) => {
    if (!tabs.some((tab) => tab.key === key)) tabs.push({ key, panel, ...labelFor(key, input) });
  };

  for (const id of INTEGRATIONS) {
    if (panelIds.includes(id)) add(id);
    if (id === "proxmox") for (const slug of categorySlugs) if (panelIds.includes(slug)) add(slug);
  }
  for (const slug of categorySlugs) if (panelIds.includes(slug)) add(slug);
  if (isDockerContainer) DOCKER_CONTAINER_TABS.forEach((id) => add(id));
  if (isDockerStack) DOCKER_STACK_TABS.forEach((id) => add(id));

  const system = systemViewsOf(panelIds, categorySlugs);
  if (system.length) tabs.push({ key: "system", panel: system[0], labelKey: "system" });
  const logs = logsViewsOf(panelIds);
  tabs.push({ key: "logs", panel: logs[0] ?? "logs", labelKey: "logs" });
  tabs.push({ key: "settings", panel: "settings", labelKey: "settings" });

  const open = tabOfPanel(activePanel, categorySlugs);
  if (activePanel && !tabs.some((tab) => tab.key === open)) add(open, activePanel);
  return tabs;
}

/** The address of a panel on this device ("/nodes/pve-01?panel=processes"); Overview has no panel. */
export function panelHref(nodeId: string, panel: string | null): string {
  const base = `/nodes/${encodeURIComponent(nodeId)}`;
  return panel ? `${base}?panel=${encodeURIComponent(panel)}` : base;
}

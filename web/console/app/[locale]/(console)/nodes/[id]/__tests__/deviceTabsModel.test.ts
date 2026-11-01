import { describe, expect, it } from "vitest";

import { buildDeviceTabs, tabOfPanel, type DeviceTabsInput } from "../deviceTabsModel";

// Links from other pages and older bookmarks use panel names (?panel=processes, ?panel=connect …). Each must open
// inside the right tab, and the tab row must always show where the page is.
const agentHost: DeviceTabsInput = {
  panelIds: ["system", "terminal", "monitoring", "processes", "network", "files", "services", "logs", "actions", "cron", "agent-settings", "connect"],
  categorySlugs: new Set(),
  isDockerContainer: false,
  isDockerStack: false,
  isProxmoxHost: false,
  activePanel: null,
};

describe("tabOfPanel", () => {
  it("puts every older panel name in its 2.0 tab", () => {
    const none = new Set<string>();
    expect(tabOfPanel(null, none)).toBe("overview");
    expect(tabOfPanel("processes", none)).toBe("system");
    expect(tabOfPanel("monitoring", none)).toBe("system");
    expect(tabOfPanel("actions", none)).toBe("logs");
    expect(tabOfPanel("connect", none)).toBe("settings");
    expect(tabOfPanel("agent-settings", none)).toBe("settings");
    expect(tabOfPanel("proxmox", none)).toBe("proxmox");
  });

  it("keeps an infra host's category lists as their own tabs, even where a System view has the same name", () => {
    expect(tabOfPanel("network", new Set(["network"]))).toBe("network");
    expect(tabOfPanel("services", new Set(["services"]))).toBe("services");
  });
});

describe("buildDeviceTabs", () => {
  it("gives an agent host Overview, System (opening Processes), Logs and Settings", () => {
    const tabs = buildDeviceTabs(agentHost);
    expect(tabs.map((tab) => tab.key)).toEqual(["overview", "system", "logs", "settings"]);
    expect(tabs.find((tab) => tab.key === "system")?.panel).toBe("processes");
  });

  it("adds Terminal at the end while it is open, and only then", () => {
    expect(buildDeviceTabs({ ...agentHost, activePanel: "terminal" }).at(-1)).toMatchObject({ key: "terminal", panel: "terminal" });
    expect(buildDeviceTabs({ ...agentHost, activePanel: "processes" }).some((tab) => tab.key === "terminal")).toBe(false);
  });

  it("lists a Proxmox host's integration and categories before System, naming its compute list Guests", () => {
    const tabs = buildDeviceTabs({
      ...agentHost,
      panelIds: ["system", "monitoring", "logs", "actions", "connect", "proxmox", "compute", "storage"],
      categorySlugs: new Set(["compute", "storage"]),
      isProxmoxHost: true,
    });
    expect(tabs.map((tab) => tab.key)).toEqual(["overview", "proxmox", "compute", "storage", "system", "logs", "settings"]);
    expect(tabs.find((tab) => tab.key === "compute")?.labelKey).toBe("guests");
    expect(tabs.find((tab) => tab.key === "system")?.panel).toBe("system");
  });
});

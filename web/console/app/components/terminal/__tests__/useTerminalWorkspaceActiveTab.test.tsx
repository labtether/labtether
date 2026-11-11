import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

// Risk: after a page reload Terminal showed the first tab, not the one that was in front, so a person working in
// their third tab landed on another device's shell.
const fake = vi.hoisted(() => ({
  loading: true,
  activeTabId: "",
  tabs: [] as Array<{ id: string; name: string; layout: string; panes: never[]; panel_sizes: null; sort_order: number }>,
  setActiveTab: vi.fn<(id: string) => void>(),
}));
vi.mock("../../../hooks/useWorkspaceTabs", () => ({
  useWorkspaceTabs: () => ({
    tabs: fake.tabs, activeTab: null, activeTabId: fake.activeTabId, setActiveTab: fake.setActiveTab,
    createTab: async () => null, updateTab: async () => {}, deleteTab: async () => {}, loading: fake.loading,
  }),
}));
vi.mock("../../../hooks/useTerminalWorkspaceTabUI", () => ({
  useTerminalWorkspaceTabUI: () => ({ handleDeleteTab: () => {}, workspaceError: null }),
}));

import { activeTabStorageKey } from "../terminalActiveTab";
import { useTerminalWorkspace } from "../useTerminalWorkspace";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
const tab = (id: string) => ({ id, name: id, layout: "single", panes: [] as never[], panel_sizes: null, sort_order: 0 });

function Workspace() {
  useTerminalWorkspace();
  return null;
}

/** The hub answers with its tabs and puts the first one in front, as after a reload. */
async function loadTabs() {
  await act(async () => root.render(<Workspace />));
  fake.tabs = [tab("tab-1"), tab("tab-2"), tab("tab-3")];
  fake.activeTabId = "tab-1";
  fake.loading = false;
  await act(async () => root.render(<Workspace />));
}

beforeEach(() => {
  window.localStorage.clear();
  fake.loading = true;
  fake.activeTabId = "";
  fake.tabs = [];
  fake.setActiveTab.mockReset();
  root = createRoot(document.createElement("div"));
});

afterEach(() => act(() => root.unmount()));

it("goes back to the tab that was in front before the reload, and remembers the next one", async () => {
  window.localStorage.setItem(activeTabStorageKey, "tab-3");
  await loadTabs();
  expect(fake.setActiveTab).toHaveBeenCalledWith("tab-3");
  expect(window.localStorage.getItem(activeTabStorageKey)).toBe("tab-3");

  // The person then brings another tab to the front.
  fake.activeTabId = "tab-2";
  await act(async () => root.render(<Workspace />));
  expect(window.localStorage.getItem(activeTabStorageKey)).toBe("tab-2");
});

it("stays on the first tab when the remembered tab was closed", async () => {
  window.localStorage.setItem(activeTabStorageKey, "tab-9");
  await loadTabs();
  expect(fake.setActiveTab).not.toHaveBeenCalled();
  expect(window.localStorage.getItem(activeTabStorageKey)).toBe("tab-1");
});

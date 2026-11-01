"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { QuickConnectParams } from "../../hooks/useSession";
import { useTerminalWorkspaceTabUI, type WorkspaceErrorCode } from "../../hooks/useTerminalWorkspaceTabUI";
import { useWorkspaceTabs, type PanelSizes, type WorkspacePane } from "../../hooks/useWorkspaceTabs";
import type { XTerminalHandle } from "../XTerminal";
import type { PaneControls, PaneStatus } from "./TerminalPane";

const EMPTY_PANES: WorkspacePane[] = [];

export function layoutPaneCount(layout: string): number {
  if (layout === "single") return 1;
  if (layout === "grid") return 4;
  return 2;
}

export const paneKey = (tabId: string, index: number) => `${tabId}:${index}`;

/** The address form: which pane it fills (null: the first free pane, or a new tab) and the address it starts with. */
export type AddressRequest = { paneKey: string | null; initial: string } | null;

/**
 * The workspace's tabs and panes as the hub stores them (each tab: a layout and one device per pane), plus what lives
 * only in this browser: the focused pane, Send to all panes, addresses typed into "Connect to an address…" (never
 * saved), each pane's status for the bar above, and the pane handles that snippets and broadcast type into.
 */
export function useTerminalWorkspace() {
  const workspace = useWorkspaceTabs();
  const { tabs, activeTabId, setActiveTab, createTab, updateTab, deleteTab } = workspace;
  const tabUI = useTerminalWorkspaceTabUI({ tabs, activeTabId, setActiveTab, createTab, updateTab, deleteTab });
  const [focusedPaneIndex, setFocusedPaneIndex] = useState(0);
  const [broadcastActive, setBroadcastActive] = useState(false);
  const [quickParams, setQuickParams] = useState<Record<string, QuickConnectParams>>({});
  const [paneStatus, setPaneStatus] = useState<Record<string, PaneStatus>>({});
  const [addressRequest, setAddressRequest] = useState<AddressRequest>(null);
  const [openError, setOpenError] = useState<WorkspaceErrorCode | null>(null);
  const paneRefs = useRef<Map<string, XTerminalHandle>>(new Map());
  const paneControls = useRef<Map<string, PaneControls>>(new Map());
  const focusedTermRef = useRef<XTerminalHandle | null>(null);

  const tabPanels = useMemo(() => tabs.map((tab) => {
    const layout = tab.layout ?? "single";
    const count = layoutPaneCount(layout);
    const panes: WorkspacePane[] = [];
    for (let index = 0; index < count; index += 1) panes.push(tab.panes?.[index] ?? { targetNodeId: "" });
    return { tabId: tab.id, layout, paneCount: count, panes, panelSizes: tab.panel_sizes };
  }), [tabs]);
  const activePanel = tabPanels.find((panel) => panel.tabId === activeTabId) ?? null;
  const layout = activePanel?.layout ?? "single";
  const paneCount = activePanel?.paneCount ?? 1;
  const panes = activePanel?.panes ?? EMPTY_PANES;
  const focusedKey = paneKey(activeTabId, focusedPaneIndex);

  /** Nothing open anywhere: Terminal shows its start page instead of an empty workspace. */
  const fresh = tabs.length === 1
    && tabPanels.every((panel) => panel.panes.every((pane, i) => !pane.targetNodeId && !quickParams[paneKey(panel.tabId, i)]));

  useEffect(() => {
    setFocusedPaneIndex((prev) => (prev >= paneCount ? 0 : prev));
  }, [paneCount, activeTabId]);

  useEffect(() => {
    focusedTermRef.current = activeTabId ? paneRefs.current.get(focusedKey) ?? null : null;
  }, [activeTabId, focusedKey]);

  useEffect(() => {
    if (!openError) return undefined;
    const timer = window.setTimeout(() => setOpenError(null), 6000);
    return () => window.clearTimeout(timer);
  }, [openError]);

  const setPaneRef = useCallback((tabId: string, index: number, handle: XTerminalHandle | null) => {
    const key = paneKey(tabId, index);
    if (handle) paneRefs.current.set(key, handle);
    else paneRefs.current.delete(key);
    if (key === focusedKey) focusedTermRef.current = handle;
  }, [focusedKey]);

  const setControls = useCallback((key: string, controls: PaneControls | null) => {
    if (controls) paneControls.current.set(key, controls);
    else paneControls.current.delete(key);
  }, []);

  const reportStatus = useCallback((key: string, status: PaneStatus) => {
    setPaneStatus((prev) => ({ ...prev, [key]: status }));
  }, []);

  const forgetAddress = useCallback((key: string) => {
    setQuickParams((prev) => {
      if (!(key in prev)) return prev;
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }, []);

  const setPaneTarget = useCallback((tabId: string, index: number, nodeId: string) => {
    const tab = tabs.find((entry) => entry.id === tabId);
    if (!tab) return;
    const count = layoutPaneCount(tab.layout ?? "single");
    const next: WorkspacePane[] = [];
    for (let i = 0; i < Math.max(count, tab.panes.length); i += 1) next.push(tab.panes[i] ?? { targetNodeId: "" });
    next[index] = { ...next[index], targetNodeId: nodeId };
    forgetAddress(paneKey(tabId, index));
    void updateTab(tabId, { panes: next });
  }, [forgetAddress, tabs, updateTab]);

  const freePane = useCallback((tabId: string) => {
    const panel = tabPanels.find((entry) => entry.tabId === tabId);
    return panel ? panel.panes.findIndex((pane, i) => !pane.targetNodeId && !quickParams[paneKey(tabId, i)]) : -1;
  }, [quickParams, tabPanels]);

  /** A tab of its own for something new; null when the hub refused it. */
  const newTab = useCallback(async () => {
    try {
      const created = await createTab();
      return created?.id ? created.id : null;
    } catch {
      setOpenError("createFailed");
      return null;
    }
  }, [createTab]);

  /**
   * Open a shell on a device: show its pane when one already has it, else use a free pane of this tab, else a new tab.
   * newTab: always a new tab.
   */
  const openTarget = useCallback(async (nodeId: string, inNewTab = false) => {
    if (!inNewTab) {
      for (const panel of tabPanels) {
        const at = panel.panes.findIndex((pane) => pane.targetNodeId === nodeId);
        if (at >= 0) {
          setActiveTab(panel.tabId);
          setFocusedPaneIndex(at);
          return;
        }
      }
      const at = activeTabId ? freePane(activeTabId) : -1;
      if (at >= 0) {
        setPaneTarget(activeTabId, at, nodeId);
        setFocusedPaneIndex(at);
        return;
      }
    }
    const id = await newTab();
    if (id) void updateTab(id, { panes: [{ targetNodeId: nodeId }] });
  }, [activeTabId, freePane, newTab, setActiveTab, setPaneTarget, tabPanels, updateTab]);

  /** The address form's result: into the pane that asked, else a free pane here, else a new tab. */
  const connectAddress = useCallback(async (params: QuickConnectParams) => {
    const request = addressRequest;
    setAddressRequest(null);
    let key = request?.paneKey ?? "";
    if (!key) {
      const at = activeTabId ? freePane(activeTabId) : -1;
      if (at >= 0) {
        key = paneKey(activeTabId, at);
        setFocusedPaneIndex(at);
      } else {
        const id = await newTab();
        if (!id) return;
        key = paneKey(id, 0);
      }
    }
    const target: string = key;
    setQuickParams((prev) => ({ ...prev, [target]: params }));
  }, [activeTabId, addressRequest, freePane, newTab]);

  const { handleDeleteTab } = tabUI;
  const closeTab = useCallback((id: string) => {
    setQuickParams((prev) => Object.fromEntries(Object.entries(prev).filter(([key]) => !key.startsWith(`${id}:`))));
    handleDeleteTab(id);
  }, [handleDeleteTab]);

  const resizePanels = useCallback((tabId: string, sizes: PanelSizes) => {
    void updateTab(tabId, { panel_sizes: sizes });
  }, [updateTab]);

  const broadcast = useCallback((data: string, source: number) => {
    if (!broadcastActive || !activeTabId) return;
    for (let index = 0; index < paneCount; index += 1) {
      if (index !== source) paneRefs.current.get(paneKey(activeTabId, index))?.sendData(data);
    }
  }, [activeTabId, broadcastActive, paneCount]);

  /** Types a snippet into the focused pane; while sending to all panes, the pane passes it on to the others. */
  const insertSnippet = useCallback((command: string) => {
    const ref = paneRefs.current.get(focusedKey);
    if (!ref) return;
    ref.sendData(command);
    ref.focus();
  }, [focusedKey]);

  const focusedControls = useCallback(() => paneControls.current.get(focusedKey) ?? null, [focusedKey]);

  return {
    ...workspace,
    tabUI,
    tabPanels,
    layout,
    paneCount,
    panes,
    fresh,
    focusedPaneIndex,
    setFocusedPaneIndex,
    focusedKey,
    focusedTermRef,
    paneStatus,
    focusedStatus: paneStatus[focusedKey] ?? null,
    focusedControls,
    broadcastActive,
    setBroadcastActive,
    quickParams,
    addressRequest,
    setAddressRequest,
    workspaceError: tabUI.workspaceError ?? openError,
    setPaneRef,
    setControls,
    reportStatus,
    setPaneTarget,
    openTarget,
    connectAddress,
    closeTab,
    resizePanels,
    broadcast,
    insertSnippet,
  };
}

export type TerminalWorkspace = ReturnType<typeof useTerminalWorkspace>;

"use client";

import { useState, useRef, useCallback, useEffect, useMemo } from "react";
import { useTranslations } from "next-intl";
import { TerminalSquare, Radio, Maximize } from "lucide-react";
import { usePaletteRegister, type PaletteProvider } from "../../../contexts/PaletteContext";
import { useWorkspaceTabs } from "../../../hooks/useWorkspaceTabs";
import { useTerminalPreferences } from "../../../hooks/useTerminalPreferences";
import { useTerminalSnippets } from "../../../hooks/useTerminalSnippets";
import { useTerminalWorkspaceTabUI } from "../../../hooks/useTerminalWorkspaceTabUI";
import SnippetPicker from "../../../components/terminal/SnippetPicker";
import SessionsPanel from "../../../components/terminal/SessionsPanel";
import ScrollbackViewer from "../../../components/terminal/ScrollbackViewer";
import { getThemeById } from "../../../terminal/themes";
import { getFontById } from "../../../terminal/fonts";
import WorkspaceGrid from "../../../components/terminal/WorkspaceGrid";
import TerminalPane from "../../../components/terminal/TerminalPane";
import KeyboardToolbar from "../../../components/terminal/KeyboardToolbar";
import SettingsPanel from "../../../components/terminal/SettingsPanel";
import TerminalWorkspaceHeader from "../../../components/terminal/TerminalWorkspaceHeader";
import type { XTerminalHandle } from "../../../components/XTerminal";
import type { WorkspacePane, PanelSizes } from "../../../hooks/useWorkspaceTabs";

const EMPTY_PANES: WorkspacePane[] = [];

function layoutPaneCount(layout: string): number {
  switch (layout) {
    case "single":
      return 1;
    case "grid":
      return 4;
    default:
      return 2;
  }
}

function paneRefKey(tabId: string, paneIndex: number): string {
  return `${tabId}:${paneIndex}`;
}

export default function TerminalWorkspacePage() {
  const t = useTranslations('terminal');
  const {
    tabs,
    activeTabId,
    setActiveTab,
    createTab,
    updateTab,
    deleteTab,
    loading: tabsLoading,
  } = useWorkspaceTabs();
  const { prefs, updatePrefs } = useTerminalPreferences();
  const { snippets } = useTerminalSnippets();
  const [fullscreen, setFullscreen] = useState(false);
  const [focusedPaneIndex, setFocusedPaneIndex] = useState(0);
  const [snippetPickerOpen, setSnippetPickerOpen] = useState(false);
  const [broadcastActive, setBroadcastActive] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [sessionsPanelOpen, setSessionsPanelOpen] = useState(false);
  const [archivedViewSession, setArchivedViewSession] = useState<{ id: string; title: string } | null>(null);

  // Refs for all terminal panes (for broadcast and tab persistence).
  const paneRefsMap = useRef<Map<string, XTerminalHandle>>(new Map());
  // Ref for focused pane (used by KeyboardToolbar).
  const focusedTermRef = useRef<XTerminalHandle | null>(null);

  const {
    layoutMenuOpen,
    setLayoutMenuOpen,
    workspaceError,
    tabMenu,
    tabMenuTarget,
    editingTabId,
    editingTabName,
    setEditingTabName,
    layoutMenuRef,
    tabMenuHostRef,
    tabRenameInputRef,
    closeTabMenu,
    openTabMenu,
    beginRenameTab,
    handleLayoutChange,
    handleCreateTab,
    handleDeleteTab,
    cancelRenameTab,
    commitRenameTab,
    handleRenameInputBlur,
    handleDuplicateTab,
    handleCloseOtherTabs,
  } = useTerminalWorkspaceTabUI({
    tabs,
    activeTabId,
    setActiveTab,
    createTab,
    updateTab,
    deleteTab,
  });

  const setPaneRef = useCallback((tabId: string, index: number, handle: XTerminalHandle | null) => {
    const key = paneRefKey(tabId, index);
    if (handle) {
      paneRefsMap.current.set(key, handle);
    } else {
      paneRefsMap.current.delete(key);
    }
    if (tabId === activeTabId && index === focusedPaneIndex) {
      focusedTermRef.current = handle;
    }
  }, [activeTabId, focusedPaneIndex]);

  // Update focused ref when focus changes.
  useEffect(() => {
    if (!activeTabId) {
      focusedTermRef.current = null;
      return;
    }
    focusedTermRef.current = paneRefsMap.current.get(paneRefKey(activeTabId, focusedPaneIndex)) ?? null;
  }, [activeTabId, focusedPaneIndex]);

  // Broadcast keyboard shortcut (Ctrl+Shift+B).
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.shiftKey && e.key === "B") {
        e.preventDefault();
        setBroadcastActive((v) => !v);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  // Snippet picker shortcut (Ctrl+Shift+S).
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.shiftKey && e.key === "S") {
        e.preventDefault();
        setSnippetPickerOpen((v) => !v);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  // Fullscreen shortcut (Ctrl+Shift+F / Escape).
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.shiftKey && e.key === "F") {
        e.preventDefault();
        setFullscreen((v) => !v);
      }
      if (e.key === "Escape" && fullscreen) {
        setFullscreen(false);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [fullscreen]);

  // Compute theme/font from prefs.
  const themeDef = useMemo(() => getThemeById(prefs.theme), [prefs.theme]);
  const fontDef = useMemo(() => getFontById(prefs.font_family), [prefs.font_family]);

  const tabPanels = useMemo(() => (
    tabs.map((tab) => {
      const layout = tab.layout ?? "single";
      const tabPaneCount = layoutPaneCount(layout);
      const existing = tab.panes ?? [];
      const panes: WorkspacePane[] = [];
      for (let index = 0; index < tabPaneCount; index += 1) {
        panes.push(existing[index] ?? { targetNodeId: "" });
      }
      return {
        tabId: tab.id,
        layout,
        paneCount: tabPaneCount,
        panes,
        panelSizes: tab.panel_sizes,
      };
    })
  ), [tabs]);
  const activeTabPanel = useMemo(
    () => tabPanels.find((panel) => panel.tabId === activeTabId) ?? null,
    [activeTabId, tabPanels],
  );
  const currentLayout = activeTabPanel?.layout ?? "single";
  const paneCount = activeTabPanel?.paneCount ?? 1;

  // Keep focus index valid when switching tabs/layouts.
  useEffect(() => {
    setFocusedPaneIndex((prev) => (prev >= paneCount ? 0 : prev));
  }, [paneCount, activeTabId]);

  // Ensure panes array matches expected count.
  const panes = activeTabPanel?.panes ?? EMPTY_PANES;

  const handlePaneTargetChange = useCallback(
    (paneIndex: number, nodeId: string) => {
      if (!activeTabId) return;
      const updated = [...panes];
      updated[paneIndex] = { ...updated[paneIndex], targetNodeId: nodeId };
      void updateTab(activeTabId, { panes: updated });
    },
    [activeTabId, panes, updateTab],
  );

  const handlePanelResize = useCallback(
    (tabId: string, sizes: PanelSizes) => {
      void updateTab(tabId, { panel_sizes: sizes });
    },
    [updateTab],
  );

  // Broadcast: when active, intercept data from focused pane and send to all other panes.
  const handleBroadcastData = useCallback(
    (data: string, sourcePaneIndex: number) => {
      if (!broadcastActive || !activeTabId) return;
      for (let index = 0; index < paneCount; index += 1) {
        if (index === sourcePaneIndex) {
          continue;
        }
        paneRefsMap.current.get(paneRefKey(activeTabId, index))?.sendData(data);
      }
    },
    [activeTabId, broadcastActive, paneCount],
  );

  const handleSnippetInsert = useCallback(
    (command: string) => {
      if (!activeTabId) {
        return;
      }
      const ref = paneRefsMap.current.get(paneRefKey(activeTabId, focusedPaneIndex));
      if (ref) {
        ref.sendData(command);
        ref.focus();
      }
    },
    [activeTabId, focusedPaneIndex],
  );

  const quickSnippet = snippets[0] ?? null;
  const handleInsertQuickSnippet = useCallback(() => {
    if (!quickSnippet) {
      return;
    }
    handleSnippetInsert(quickSnippet.command);
  }, [handleSnippetInsert, quickSnippet]);

  // Session panel: select an active/detached persistent session.
  const handleSessionSelect = useCallback(
    async (_sessionId: string, _persistentSessionId: string, _streamTicket: string, newTab: boolean) => {
      // When newTab is requested, create a fresh tab first.
      // For now the simplest integration is to set the focused pane's target to
      // trigger the existing connection flow. The persistent-session-level
      // reconnect will be refined in a later pass.
      if (newTab) {
        try {
          await createTab();
        } catch {
          // If tab creation fails, fall through to the current tab.
        }
      }
      // The session attach API has already been called by SessionsPanel.
      // A future refinement will pass the stream ticket / persistent session ID
      // into TerminalPane for direct reconnection. For now the panel click gives
      // the user a visual indication and the session is tracked server-side.
    },
    [createTab],
  );

  // Session panel: connect via a saved bookmark.
  const handleBookmarkConnect = useCallback(
    async (_bookmarkId: string, newTab: boolean) => {
      if (newTab) {
        try {
          await createTab();
        } catch {
          // Fall through to current tab on failure.
        }
      }
    },
    [createTab],
  );

  const terminalPaletteProvider = useMemo((): PaletteProvider => ({
    id: "terminal-commands",
    group: t('palette.group'),
    priority: 1,
    search(query: string) {
      const items = [
        {
          id: "term-new-tab",
          label: t('palette.newTab'),
          icon: TerminalSquare,
          action: () => handleCreateTab(),
          keywords: ["tab", "new", "create"],
        },
        {
          id: "term-broadcast",
          label: broadcastActive ? t('palette.disableBroadcast') : t('palette.enableBroadcast'),
          icon: Radio,
          action: () => setBroadcastActive((v: boolean) => !v),
          keywords: ["broadcast", "sync", "multi"],
        },
        {
          id: "term-fullscreen",
          label: fullscreen ? t('palette.exitFullscreen') : t('palette.enterFullscreen'),
          icon: Maximize,
          action: () => setFullscreen((v: boolean) => !v),
          keywords: ["fullscreen", "maximize", "expand"],
        },
      ];
      if (!query.trim()) return items;
      const lower = query.toLowerCase();
      return items.filter(
        (item) =>
          item.label.toLowerCase().includes(lower) ||
          (item.keywords ?? []).some((kw) => kw.includes(lower))
      );
    },
  }), [broadcastActive, fullscreen, handleCreateTab, t]);

  usePaletteRegister(terminalPaletteProvider);

  if (tabsLoading) {
    return (
      <div className="fixed inset-0 z-10 md:left-52">
        <div className="flex h-full items-center justify-center bg-[var(--panel)]">
          <p className="text-sm text-[var(--muted)]">{t('loading')}</p>
        </div>
      </div>
    );
  }

  return (
    <>
      <div className={fullscreen ? "fixed inset-0 z-50" : "fixed inset-0 z-10 md:left-52"}>
        <div
          ref={tabMenuHostRef}
          className="flex h-full flex-col overflow-hidden bg-[var(--panel)]"
        >
          <h1 className="sr-only">{t('title')}</h1>
          <TerminalWorkspaceHeader
            tabs={tabs}
            activeTabId={activeTabId}
            editingTabId={editingTabId}
            editingTabName={editingTabName}
            tabRenameInputRef={tabRenameInputRef}
            layoutMenuRef={layoutMenuRef}
            layoutMenuOpen={layoutMenuOpen}
            currentLayout={currentLayout}
            paneCount={paneCount}
            broadcastActive={broadcastActive}
            fullscreen={fullscreen}
            tabMenu={tabMenu}
            tabMenuTarget={tabMenuTarget}
            quickSnippetName={quickSnippet?.name ?? null}
            onInsertQuickSnippet={quickSnippet ? handleInsertQuickSnippet : null}
            onEditingTabNameChange={setEditingTabName}
            onSetActiveTab={setActiveTab}
            onOpenTabMenu={openTabMenu}
            onCreateTab={handleCreateTab}
            onDeleteTab={handleDeleteTab}
            onToggleLayoutMenu={() => setLayoutMenuOpen((v) => !v)}
            onLayoutChange={handleLayoutChange}
            onToggleBroadcast={() => setBroadcastActive((v) => !v)}
            onOpenSnippetPicker={() => setSnippetPickerOpen(true)}
            onToggleFullscreen={() => setFullscreen((v) => !v)}
            onToggleSettings={() => setSettingsOpen((v) => !v)}
            onCloseTabMenu={closeTabMenu}
            onBeginRenameTab={beginRenameTab}
            onCommitRenameTab={commitRenameTab}
            onCancelRenameTab={cancelRenameTab}
            onRenameInputBlur={handleRenameInputBlur}
            onDuplicateTab={handleDuplicateTab}
            onCloseOtherTabs={handleCloseOtherTabs}
            sessionsPanelOpen={sessionsPanelOpen}
            onToggleSessionsPanel={() => setSessionsPanelOpen((v) => !v)}
          />

          {workspaceError ? (
            <div className="border-b border-[var(--bad)]/35 bg-[var(--bad-glow)] px-3 py-2 text-xs text-[var(--bad)]">
              {workspaceError}
            </div>
          ) : null}

          {/* Main content area */}
          <div className="flex min-h-0 flex-1 overflow-hidden bg-[var(--bg)]/25">
            <SessionsPanel
              isOpen={sessionsPanelOpen}
              onClose={() => setSessionsPanelOpen(false)}
              onSessionSelect={handleSessionSelect}
              onBookmarkConnect={handleBookmarkConnect}
              onArchivedView={(id, title) => setArchivedViewSession({ id, title })}
            />
            <div className="flex min-w-0 flex-1 flex-col">
              <div className="relative flex min-h-0 flex-1 overflow-hidden">
                {tabPanels.map((panel) => {
                  const isActive = panel.tabId === activeTabId;
                  return (
                    <div
                      key={panel.tabId}
                      className={isActive
                        ? "flex min-w-0 flex-1 flex-col"
                        : "pointer-events-none absolute inset-0 flex min-w-0 flex-col opacity-0"}
                      aria-hidden={!isActive}
                    >
                      <WorkspaceGrid
                        layout={panel.layout}
                        paneCount={panel.paneCount}
                        panelSizes={panel.panelSizes}
                        onPanelResize={(sizes) => handlePanelResize(panel.tabId, sizes)}
                      >
                        {panel.panes.slice(0, panel.paneCount).map((pane, idx) => (
                          <TerminalPane
                            key={`${panel.tabId}-pane-${idx}`}
                            paneIndex={idx}
                            targetNodeId={pane.targetNodeId}
                            onTargetChange={(nodeId) => {
                              if (isActive) {
                                handlePaneTargetChange(idx, nodeId);
                              }
                            }}
                            isTabActive={isActive}
                            isFocused={isActive && focusedPaneIndex === idx}
                            onFocus={() => {
                              if (!isActive) {
                                setActiveTab(panel.tabId);
                              }
                              setFocusedPaneIndex(idx);
                            }}
                            broadcastActive={broadcastActive && isActive}
                            onBroadcastData={handleBroadcastData}
                            onTerminalRef={(handle) => setPaneRef(panel.tabId, idx, handle)}
                            prefs={prefs}
                            themeDef={themeDef}
                            fontDef={fontDef}
                          />
                        ))}
                      </WorkspaceGrid>
                    </div>
                  );
                })}
              </div>

              <KeyboardToolbar
                termRef={focusedTermRef}
                keys={prefs.toolbar_keys ?? undefined}
              />
            </div>
          </div>
        </div>
      </div>

      <SettingsPanel
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        prefs={prefs}
        onUpdatePrefs={updatePrefs}
      />

      <SnippetPicker
        snippets={snippets}
        open={snippetPickerOpen}
        onClose={() => setSnippetPickerOpen(false)}
        onSelect={handleSnippetInsert}
      />

      {archivedViewSession && (
        <ScrollbackViewer
          persistentSessionId={archivedViewSession.id}
          title={archivedViewSession.title}
          onClose={() => setArchivedViewSession(null)}
        />
      )}
    </>
  );
}

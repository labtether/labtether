"use client";

import type { MouseEvent as ReactMouseEvent, RefObject } from "react";
import type { WorkspaceTab } from "../../hooks/useWorkspaceTabs";
import type { TabContextMenuState } from "../../hooks/useTerminalWorkspaceTabUI";
import {
  Plus,
  X,
  LayoutGrid,
  Radio,
  Settings,
  Columns,
  Rows,
  Square,
  PanelLeft,
  PanelBottom,
  Code,
  Maximize,
  Minimize,
  PanelLeftOpen,
  PanelLeftClose,
} from "lucide-react";

const layoutOptions = [
  { id: "single", label: "Single", icon: Square },
  { id: "columns", label: "Columns", icon: Columns },
  { id: "rows", label: "Rows", icon: Rows },
  { id: "grid", label: "Grid", icon: LayoutGrid },
  { id: "main-side", label: "Main + Side", icon: PanelLeft },
  { id: "main-bottom", label: "Main + Bottom", icon: PanelBottom },
] as const;

const iconButtonClass =
  "inline-flex h-8 w-8 items-center justify-center rounded-md border border-transparent text-[var(--muted)] transition-colors hover:border-[var(--line)] hover:bg-[var(--hover)] hover:text-[var(--text)]";

const contextMenuItemClass =
  "flex w-full items-center rounded px-3 py-1.5 text-left text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]";

const contextMenuItemDisabledClass =
  "flex w-full cursor-default items-center rounded px-3 py-1.5 text-left text-xs text-[var(--muted)] opacity-60";

type TerminalWorkspaceHeaderProps = {
  tabs: WorkspaceTab[];
  activeTabId: string;
  editingTabId: string | null;
  editingTabName: string;
  tabRenameInputRef: RefObject<HTMLInputElement | null>;
  layoutMenuRef: RefObject<HTMLDivElement | null>;
  layoutMenuOpen: boolean;
  currentLayout: string;
  paneCount: number;
  broadcastActive: boolean;
  fullscreen: boolean;
  tabMenu: TabContextMenuState | null;
  tabMenuTarget: WorkspaceTab | null;
  onEditingTabNameChange: (name: string) => void;
  onSetActiveTab: (id: string) => void;
  onOpenTabMenu: (event: ReactMouseEvent, tabId: string) => void;
  onCreateTab: () => void;
  onDeleteTab: (id: string) => void;
  onToggleLayoutMenu: () => void;
  onLayoutChange: (layout: string) => void;
  onToggleBroadcast: () => void;
  quickSnippetName: string | null;
  onInsertQuickSnippet: (() => void) | null;
  onOpenSnippetPicker: () => void;
  onToggleFullscreen: () => void;
  onToggleSettings: () => void;
  onCloseTabMenu: () => void;
  onBeginRenameTab: (tab: WorkspaceTab) => void;
  onCommitRenameTab: () => Promise<void>;
  onCancelRenameTab: () => void;
  onRenameInputBlur: () => void;
  onDuplicateTab: (tab: WorkspaceTab) => Promise<void>;
  onCloseOtherTabs: (tabId: string) => Promise<void>;
  sessionsPanelOpen?: boolean;
  onToggleSessionsPanel?: () => void;
};

export default function TerminalWorkspaceHeader({
  tabs,
  activeTabId,
  editingTabId,
  editingTabName,
  tabRenameInputRef,
  layoutMenuRef,
  layoutMenuOpen,
  currentLayout,
  paneCount,
  broadcastActive,
  fullscreen,
  tabMenu,
  tabMenuTarget,
  onEditingTabNameChange,
  onSetActiveTab,
  onOpenTabMenu,
  onCreateTab,
  onDeleteTab,
  onToggleLayoutMenu,
  onLayoutChange,
  onToggleBroadcast,
  quickSnippetName,
  onInsertQuickSnippet,
  onOpenSnippetPicker,
  onToggleFullscreen,
  onToggleSettings,
  onCloseTabMenu,
  onBeginRenameTab,
  onCommitRenameTab,
  onCancelRenameTab,
  onRenameInputBlur,
  onDuplicateTab,
  onCloseOtherTabs,
  sessionsPanelOpen,
  onToggleSessionsPanel,
}: TerminalWorkspaceHeaderProps) {
  const layoutLabel = layoutOptions.find((opt) => opt.id === currentLayout)?.label ?? "Single";

  return (
    <>
      <div className="relative z-[80] flex items-center gap-2 border-b border-[var(--line)] bg-[var(--panel-glass)] px-2 py-2 backdrop-blur-[16px]">
        <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
          {tabs.map((tab) => (
            <div
              key={tab.id}
              role="button"
              tabIndex={0}
              data-tab-id={tab.id}
              onClick={() => {
                if (editingTabId === tab.id) return;
                onSetActiveTab(tab.id);
              }}
              onContextMenu={(event) => onOpenTabMenu(event, tab.id)}
              onKeyDown={(event) => {
                if (editingTabId === tab.id) return;
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  onSetActiveTab(tab.id);
                }
              }}
              className={`group inline-flex h-8 cursor-pointer items-center gap-2 rounded-md border px-3 text-xs transition-colors ${
                tab.id === activeTabId
                  ? "border-[var(--accent)]/40 border-b-2 border-b-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--text)]"
                  : "border-transparent bg-transparent text-[var(--muted)] hover:border-[var(--line)] hover:bg-[var(--hover)] hover:text-[var(--text)]"
              }`}
              style={
                tab.id === activeTabId
                  ? { boxShadow: "0 0 8px var(--accent-glow)" }
                  : undefined
              }
            >
              {editingTabId === tab.id ? (
                <input
                  ref={tabRenameInputRef}
                  value={editingTabName}
                  maxLength={48}
                  onChange={(event) => onEditingTabNameChange(event.target.value)}
                  onClick={(event) => event.stopPropagation()}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") {
                      event.preventDefault();
                      void onCommitRenameTab();
                    } else if (event.key === "Escape") {
                      event.preventDefault();
                      onCancelRenameTab();
                    }
                  }}
                  onBlur={onRenameInputBlur}
                  className="h-6 w-[150px] rounded border border-[var(--line)] bg-[var(--surface)] px-2 text-xs text-[var(--text)] outline-none transition-colors focus:border-[var(--accent)]"
                />
              ) : (
                <span className="max-w-[130px] truncate">{tab.name}</span>
              )}
              {tabs.length > 1 && editingTabId !== tab.id ? (
                <button
                  type="button"
                  onClick={(event) => {
                    event.stopPropagation();
                    onDeleteTab(tab.id);
                  }}
                  className="inline-flex h-4 w-4 items-center justify-center rounded text-[var(--muted)] transition-colors hover:bg-[var(--surface)] hover:text-[var(--text)]"
                  aria-label={`Close ${tab.name}`}
                >
                  <X size={10} />
                </button>
              ) : null}
            </div>
          ))}
          <button
            type="button"
            onClick={onCreateTab}
            className={iconButtonClass}
            title="New tab"
          >
            <Plus size={14} />
          </button>
        </div>

        <div className="hidden items-center gap-2 md:flex">
          <span className="rounded-full border border-[var(--line)] bg-[var(--panel-glass)] px-2 py-0.5 text-[10px] uppercase tracking-[0.06em] text-[var(--muted)] backdrop-blur-[12px]">
            {layoutLabel}
          </span>
          <span className="rounded-full border border-[var(--line)] bg-[var(--panel-glass)] px-2 py-0.5 text-[10px] uppercase tracking-[0.06em] text-[var(--muted)] backdrop-blur-[12px]">
            {paneCount} pane{paneCount > 1 ? "s" : ""}
          </span>
          {broadcastActive ? (
            <span className="rounded-full border border-[var(--warn)]/45 bg-[var(--warn-glow)] px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-[var(--warn)]">
              Broadcast
            </span>
          ) : null}
        </div>

        <div className="flex items-center gap-1">
          {onToggleSessionsPanel ? (
            <button
              type="button"
              onClick={onToggleSessionsPanel}
              className={`${iconButtonClass} ${
                sessionsPanelOpen
                  ? "border-[var(--accent)]/40 bg-[var(--accent-subtle)] text-[var(--accent)]"
                  : ""
              }`}
              title={sessionsPanelOpen ? "Hide sessions panel" : "Show sessions panel"}
            >
              {sessionsPanelOpen ? <PanelLeftClose size={14} /> : <PanelLeftOpen size={14} />}
            </button>
          ) : null}

          <div ref={layoutMenuRef} className="relative">
            <button
              type="button"
              onClick={onToggleLayoutMenu}
              className={`${iconButtonClass} ${layoutMenuOpen ? "border-[var(--line)] bg-[var(--hover)] text-[var(--text)]" : ""}`}
              title="Change layout"
            >
              <LayoutGrid size={14} />
            </button>
            {layoutMenuOpen ? (
              <div className="absolute right-0 top-full z-[80] mt-2 min-w-[170px] rounded-lg border border-[var(--line)] bg-[var(--panel-glass)] p-1 shadow-[var(--shadow-md)] backdrop-blur-[16px]">
                {layoutOptions.map((opt) => {
                  const Icon = opt.icon;
                  const active = currentLayout === opt.id;
                  return (
                    <button
                      key={opt.id}
                      type="button"
                      onClick={() => onLayoutChange(opt.id)}
                      className={`flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs transition-colors ${
                        active
                          ? "bg-[var(--accent-subtle)] text-[var(--text)]"
                          : "text-[var(--muted)] hover:bg-[var(--hover)] hover:text-[var(--text)]"
                      }`}
                    >
                      <Icon size={13} />
                      <span>{opt.label}</span>
                    </button>
                  );
                })}
              </div>
            ) : null}
          </div>

          <button
            type="button"
            onClick={onToggleBroadcast}
            className={`${iconButtonClass} ${
              broadcastActive
                ? "border-[var(--warn)]/45 bg-[var(--warn-glow)] text-[var(--warn)]"
                : ""
            }`}
            title={broadcastActive ? "Broadcast ON (Ctrl+Shift+B)" : "Broadcast OFF (Ctrl+Shift+B)"}
          >
            <Radio size={14} />
          </button>

          {quickSnippetName && onInsertQuickSnippet ? (
            <button
              type="button"
              onClick={onInsertQuickSnippet}
              className={iconButtonClass}
              title={`Insert ${quickSnippetName}`}
            >
              <Code size={14} />
            </button>
          ) : null}

          <button
            type="button"
            onClick={onOpenSnippetPicker}
            className={iconButtonClass}
            title="Snippets (Ctrl+Shift+S)"
          >
            <Code size={14} />
          </button>

          <button
            type="button"
            onClick={onToggleFullscreen}
            className={iconButtonClass}
            title={fullscreen ? "Exit fullscreen (Ctrl+Shift+F)" : "Fullscreen (Ctrl+Shift+F)"}
          >
            {fullscreen ? <Minimize size={14} /> : <Maximize size={14} />}
          </button>

          <button
            type="button"
            onClick={onToggleSettings}
            className={iconButtonClass}
            title="Terminal settings"
          >
            <Settings size={14} />
          </button>
        </div>
      </div>

      {tabMenu && tabMenuTarget ? (
        <div
          className="absolute inset-0 z-[90]"
          onClick={onCloseTabMenu}
          onContextMenu={(event) => {
            event.preventDefault();
            onCloseTabMenu();
          }}
        >
          <div
            className="absolute min-w-[220px] rounded-lg border border-[var(--line)] bg-[var(--panel-glass)] p-1 shadow-xl backdrop-blur-[16px]"
            style={{ left: tabMenu.x, top: tabMenu.y }}
            onClick={(event) => event.stopPropagation()}
          >
            <button
              type="button"
              className={contextMenuItemClass}
              onClick={() => {
                onBeginRenameTab(tabMenuTarget);
              }}
            >
              Rename Tab
            </button>
            <button
              type="button"
              className={contextMenuItemClass}
              onClick={() => {
                onCloseTabMenu();
                void onDuplicateTab(tabMenuTarget);
              }}
            >
              Duplicate Tab
            </button>
            <button
              type="button"
              className={contextMenuItemClass}
              onClick={() => {
                onCloseTabMenu();
                onCreateTab();
              }}
            >
              New Tab
            </button>
            <div className="my-1 border-t border-[var(--line)]" />
            <button
              type="button"
              className={tabs.length > 1 ? contextMenuItemClass : contextMenuItemDisabledClass}
              disabled={tabs.length <= 1}
              onClick={() => {
                onCloseTabMenu();
                onDeleteTab(tabMenuTarget.id);
              }}
            >
              Close Tab
            </button>
            <button
              type="button"
              className={tabs.length > 1 ? contextMenuItemClass : contextMenuItemDisabledClass}
              disabled={tabs.length <= 1}
              onClick={() => {
                onCloseTabMenu();
                void onCloseOtherTabs(tabMenuTarget.id);
              }}
            >
              Close Other Tabs
            </button>
          </div>
        </div>
      ) : null}
    </>
  );
}
